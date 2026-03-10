package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/makemoments/gorender/internal/composition"
	"github.com/makemoments/gorender/internal/pipeline"
	"go.uber.org/zap"
)

// Writer pipes ordered PNG frames into an ffmpeg process.
// Frames are streamed via stdin (image2pipe) — no disk accumulation needed
// beyond what the scheduler has already written.
type Writer struct {
	comp *composition.Composition
	log  *zap.Logger
}

// NewWriter creates an FFmpeg Writer for the given composition.
func NewWriter(comp *composition.Composition, log *zap.Logger) *Writer {
	return &Writer{comp: comp, log: log}
}

// Write consumes the ordered frame channel and pipes them into ffmpeg.
// Audio tracks in comp.Audio are muxed in after the video pass.
func (w *Writer) Write(ctx context.Context, frames <-chan pipeline.FrameInOrder) error {
	// Step 1: encode video-only from piped PNG frames.
	videoPath, err := w.encodeVideo(ctx, frames)
	if err != nil {
		return fmt.Errorf("encoding video: %w", err)
	}
	defer os.Remove(videoPath) // clean up temp video

	// Step 2: mux audio if any tracks are defined.
	if len(w.comp.Audio) == 0 {
		return os.Rename(videoPath, w.comp.Output.Path)
	}
	return w.muxAudio(ctx, videoPath)
}

// encodeVideo streams PNGs into ffmpeg and produces a temporary video file.
func (w *Writer) encodeVideo(ctx context.Context, frames <-chan pipeline.FrameInOrder) (string, error) {
	tmpVideo := w.comp.Output.Path + ".tmp.mp4"

	args := []string{
		"-y",                    // overwrite output
		"-f", "image2pipe",      // input format: piped images
		"-framerate", fmt.Sprintf("%d", w.comp.FPS),
		"-i", "pipe:0",          // read from stdin
		"-c:v", w.videoCodec(),
		"-preset", w.comp.Output.Preset,
		"-crf", fmt.Sprintf("%d", w.comp.Output.CRF),
		"-pix_fmt", w.comp.Output.PixelFormat,
		"-movflags", "+faststart", // web-optimized MP4
		tmpVideo,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = &zapWriter{log: w.log, prefix: "[ffmpeg] "}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting ffmpeg: %w", err)
	}

	// Stream frames into ffmpeg stdin.
	var writeErr error
	for frame := range frames {
		if writeErr != nil {
			continue // drain channel even on error
		}
		data, err := os.ReadFile(frame.Path)
		if err != nil {
			writeErr = fmt.Errorf("reading frame %d: %w", frame.Frame, err)
			continue
		}
		if _, err := stdin.Write(data); err != nil {
			writeErr = fmt.Errorf("writing frame %d to ffmpeg: %w", frame.Frame, err)
		}
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg exited: %w", err)
	}
	if writeErr != nil {
		return "", writeErr
	}

	w.log.Info("video encoded", zap.String("path", tmpVideo))
	return tmpVideo, nil
}

// muxAudio combines the video track with one or more audio tracks.
func (w *Writer) muxAudio(ctx context.Context, videoPath string) error {
	args := []string{"-y", "-i", videoPath}

	// Add each audio track as an input.
	for _, track := range w.comp.Audio {
		args = append(args, "-i", track.Src)
	}

	// Build the audio filter graph.
	// Trims, delays (based on startFrame), and volume adjustments per track,
	// then amix all tracks together.
	filterParts := make([]string, 0, len(w.comp.Audio)+1)
	mixInputs := make([]string, 0, len(w.comp.Audio))

	for i, track := range w.comp.Audio {
		inputIdx := i + 1 // video is input 0
		label := fmt.Sprintf("a%d", i)

		startSec := float64(track.StartFrame) / float64(w.comp.FPS)
		vol := track.Volume
		if vol == 0 {
			vol = 1.0
		}

		filter := fmt.Sprintf("[%d:a]", inputIdx)

		// Trim if specified.
		if track.TrimStart > 0 || track.TrimEnd > 0 {
			trimEnd := ""
			if track.TrimEnd > 0 {
				trimEnd = fmt.Sprintf(":end=%f", track.TrimEnd.Seconds())
			}
			filter += fmt.Sprintf("atrim=start=%f%s,asetpts=PTS-STARTPTS,",
				track.TrimStart.Seconds(), trimEnd)
		}

		// Delay to startFrame.
		if startSec > 0 {
			filter += fmt.Sprintf("adelay=%d:all=1,", int(startSec*1000))
		}

		// Volume.
		filter += fmt.Sprintf("volume=%f[%s]", vol, label)

		filterParts = append(filterParts, filter)
		mixInputs = append(mixInputs, fmt.Sprintf("[%s]", label))
	}

	// Mix all audio tracks.
	mixFilter := strings.Join(mixInputs, "") +
		fmt.Sprintf("amix=inputs=%d:duration=first:dropout_transition=0[aout]", len(w.comp.Audio))
	filterParts = append(filterParts, mixFilter)

	args = append(args,
		"-filter_complex", strings.Join(filterParts, ";"),
		"-map", "0:v",
		"-map", "[aout]",
		"-c:v", "copy", // don't re-encode video
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		w.comp.Output.Path,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = &zapWriter{log: w.log, prefix: "[ffmpeg-audio] "}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("muxing audio: %w", err)
	}

	w.log.Info("audio muxed", zap.String("output", w.comp.Output.Path))
	return nil
}

// videoCodec returns the appropriate codec for the output format.
func (w *Writer) videoCodec() string {
	if w.comp.Output.VideoCodec != "" {
		return w.comp.Output.VideoCodec
	}
	switch w.comp.Output.Format {
	case "webm":
		return "libvpx-vp9"
	case "gif":
		return "gif"
	default:
		return "libx264"
	}
}

// EnsureOutputDir creates the output directory if it doesn't exist.
func EnsureOutputDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}

// zapWriter bridges io.Writer to zap, line-buffering stderr from ffmpeg.
type zapWriter struct {
	log    *zap.Logger
	prefix string
	buf    strings.Builder
}

func (z *zapWriter) Write(p []byte) (n int, err error) {
	z.buf.Write(p)
	for {
		s := z.buf.String()
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(s[:idx], "\r")
		if line != "" {
			z.log.Debug(z.prefix + line)
		}
		z.buf.Reset()
		z.buf.WriteString(s[idx+1:])
	}
	return len(p), nil
}

// Probe runs ffprobe on a file and returns basic info as a map.
// Useful for verifying output duration, codec, etc.
func Probe(path string) (map[string]string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "flat",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
		}
	}
	return result, nil
}

// Check verifies ffmpeg and ffprobe are available in PATH.
func Check() error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found in PATH: %w", bin, err)
		}
	}
	return nil
}

// Ensure io.Writer is implemented.
var _ io.Writer = (*zapWriter)(nil)
