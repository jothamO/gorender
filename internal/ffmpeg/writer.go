package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/makemoments/gorender/internal/composition"
	"github.com/makemoments/gorender/internal/pipeline"
	"go.uber.org/zap"
)

// Writer pipes ordered PNG frames into an ffmpeg process.
// Frames are streamed via stdin (image2pipe) — no disk accumulation needed
// beyond what the scheduler has already written.
type Writer struct {
	comp         *composition.Composition
	inputFormat  string
	experimental bool
	log          *zap.Logger
}

// NewWriter creates an FFmpeg Writer for the given composition.
func NewWriter(comp *composition.Composition, inputFormat string, log *zap.Logger, experimental ...bool) *Writer {
	if inputFormat == "" {
		inputFormat = "png"
	}
	exp := len(experimental) > 0 && experimental[0]
	return &Writer{comp: comp, inputFormat: inputFormat, experimental: exp, log: log}
}

// Write consumes the ordered frame channel and pipes them into ffmpeg.
// Audio tracks in comp.Audio are muxed in after the video pass.
func (w *Writer) Write(ctx context.Context, frames <-chan pipeline.FrameInOrder) error {
	totalStart := time.Now()
	// Step 1: encode video-only from piped PNG frames.
	encodeStart := time.Now()
	videoPath, err := w.encodeVideo(ctx, frames)
	encodeDur := time.Since(encodeStart)
	if err != nil {
		return fmt.Errorf("encoding video: %w", err)
	}
	defer os.Remove(videoPath) // clean up temp video

	// Step 2: optional upscale pass.
	upscaleDur := time.Duration(0)
	if w.shouldUpscale() && !w.shouldInlineUpscale() {
		upscaleStart := time.Now()
		upscaledPath, uerr := w.upscaleVideo(ctx, videoPath)
		if uerr != nil {
			return fmt.Errorf("upscaling video: %w", uerr)
		}
		upscaleDur = time.Since(upscaleStart)
		defer os.Remove(upscaledPath)
		videoPath = upscaledPath
	}

	// Step 3: mux audio if any tracks are defined.
	muxStart := time.Now()
	if len(w.comp.Audio) == 0 {
		if err := os.Rename(videoPath, w.comp.Output.Path); err != nil {
			return err
		}
		w.log.Info("ffmpeg timing split",
			zap.Duration("encode", encodeDur),
			zap.Duration("upscale", upscaleDur),
			zap.Duration("mux", 0),
			zap.Duration("total", time.Since(totalStart)),
		)
		return nil
	}
	if err := w.muxAudio(ctx, videoPath); err != nil {
		return err
	}
	muxDur := time.Since(muxStart)
	w.log.Info("ffmpeg timing split",
		zap.Duration("encode", encodeDur),
		zap.Duration("upscale", upscaleDur),
		zap.Duration("mux", muxDur),
		zap.Duration("total", time.Since(totalStart)),
	)
	return nil
}

// encodeVideo streams PNGs into ffmpeg and produces a temporary video file.
func (w *Writer) encodeVideo(ctx context.Context, frames <-chan pipeline.FrameInOrder) (string, error) {
	tmpVideo := w.comp.Output.Path + ".tmp.mp4"

	args := []string{"-y", "-f", "image2pipe", "-framerate", fmt.Sprintf("%d", w.comp.FPS)}
	if w.inputFormat == "jpeg" {
		args = append(args, "-vcodec", "mjpeg")
	}
	args = append(args,
		"-i", "pipe:0",
	)
	if w.shouldInlineUpscale() {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", w.comp.Output.UpscaleWidth, w.comp.Output.UpscaleHeight))
	}
	args = append(args,
		"-c:v", w.videoCodec(),
		"-preset", w.comp.Output.Preset,
		"-crf", fmt.Sprintf("%d", w.comp.Output.CRF),
		"-pix_fmt", w.comp.Output.PixelFormat,
		"-movflags", "+faststart",
		tmpVideo,
	)

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
	// Supports sparse incoming frames by repeating the most recent frame bytes
	// to fill any gaps (used by frame-step rendering).
	var writeErr error
	nextFrame := 0
	var last []byte
	for frame := range frames {
		if writeErr != nil {
			continue // drain channel even on error
		}
		data := frame.Bytes
		if len(data) == 0 {
			readData, err := os.ReadFile(frame.Path)
			if err != nil {
				writeErr = fmt.Errorf("reading frame %d: %w", frame.Frame, err)
				continue
			}
			data = readData
		}

		if frame.Frame > nextFrame {
			if len(last) == 0 {
				writeErr = fmt.Errorf("missing prior frame for gap before frame %d", frame.Frame)
				continue
			}
			for nextFrame < frame.Frame {
				if _, err := stdin.Write(last); err != nil {
					writeErr = fmt.Errorf("writing repeated frame %d to ffmpeg: %w", nextFrame, err)
					break
				}
				nextFrame++
			}
			if writeErr != nil {
				continue
			}
		}

		if _, err := stdin.Write(data); err != nil {
			writeErr = fmt.Errorf("writing frame %d to ffmpeg: %w", frame.Frame, err)
			continue
		}
		last = data
		nextFrame = frame.Frame + 1
	}
	if writeErr == nil && len(last) > 0 {
		for nextFrame < w.comp.DurationFrames {
			if _, err := stdin.Write(last); err != nil {
				writeErr = fmt.Errorf("writing tail frame %d to ffmpeg: %w", nextFrame, err)
				break
			}
			nextFrame++
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

func (w *Writer) shouldUpscale() bool {
	uw := w.comp.Output.UpscaleWidth
	uh := w.comp.Output.UpscaleHeight
	if uw <= 0 || uh <= 0 {
		return false
	}
	if uw == w.comp.Width && uh == w.comp.Height {
		return false
	}
	return w.comp.Output.Format == "" || w.comp.Output.Format == "mp4"
}

func (w *Writer) shouldInlineUpscale() bool {
	// Experimental path: merge upscale into the primary encode to avoid a second full encode pass.
	return w.experimental && w.shouldUpscale()
}

func (w *Writer) upscaleVideo(ctx context.Context, videoPath string) (string, error) {
	out := w.comp.Output.Path + ".upscale.tmp.mp4"
	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", w.comp.Output.UpscaleWidth, w.comp.Output.UpscaleHeight),
		"-c:v", w.videoCodec(),
		"-preset", w.comp.Output.Preset,
		"-crf", fmt.Sprintf("%d", w.comp.Output.CRF),
		"-pix_fmt", w.comp.Output.PixelFormat,
		"-an",
		"-movflags", "+faststart",
		out,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = &zapWriter{log: w.log, prefix: "[ffmpeg-upscale] "}
	if err := cmd.Run(); err != nil {
		return "", err
	}
	w.log.Info("video upscaled",
		zap.Int("width", w.comp.Output.UpscaleWidth),
		zap.Int("height", w.comp.Output.UpscaleHeight),
		zap.String("path", out),
	)
	return out, nil
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
