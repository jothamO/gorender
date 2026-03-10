package composition

import "time"

// Composition is the contract between gorender and any web frontend.
// The frontend must read ?frame=N (or whatever SeekParam is set to)
// and render deterministically for that frame value.
type Composition struct {
	// URL of the web composition. Can be localhost or remote.
	URL string `json:"url" yaml:"url"`

	// DurationFrames is the total number of frames to render.
	DurationFrames int `json:"durationFrames" yaml:"durationFrames"`

	// FPS is frames per second. Typically 24 or 30.
	FPS int `json:"fps" yaml:"fps"`

	// Width and Height of the output video in pixels.
	Width  int `json:"width" yaml:"width"`
	Height int `json:"height" yaml:"height"`

	// SeekParam is the query parameter name used to inject the current frame.
	// Defaults to "frame". e.g. https://yourapp.com/comp?frame=42
	SeekParam string `json:"seekParam,omitempty" yaml:"seekParam,omitempty"`

	// ReadySignal is a JS expression evaluated in the page that must return
	// true before a screenshot is taken. Defaults to "window.__READY__ === true".
	// Your composition should set window.__READY__ = true once all assets
	// are loaded and the frame is painted.
	ReadySignal string `json:"readySignal,omitempty" yaml:"readySignal,omitempty"`

	// ReadyTimeout is how long to wait for ReadySignal before giving up.
	// Defaults to 5s.
	ReadyTimeout time.Duration `json:"readyTimeout,omitempty" yaml:"readyTimeout,omitempty"`

	// Audio tracks to mix into the final output.
	Audio []AudioTrack `json:"audio,omitempty" yaml:"audio,omitempty"`

	// Output configuration.
	Output OutputConfig `json:"output" yaml:"output"`

	// SlideDurationsMs optionally carries per-slide durations used to derive
	// deterministic timeline hints during rendering.
	SlideDurationsMs []int `json:"slideDurationsMs,omitempty" yaml:"slideDurationsMs,omitempty"`

	// EmitTimelineQuery enables adding derived timeline query params per frame:
	// gr_slide, gr_in_slide_ms, gr_slide_ms, gr_t.
	// This is guarded/off by default to avoid changing existing frontend behavior.
	EmitTimelineQuery bool `json:"emitTimelineQuery,omitempty" yaml:"emitTimelineQuery,omitempty"`
}

// AudioTrack describes a single audio file to be mixed into the output.
type AudioTrack struct {
	// Src is a path or URL to the audio file (mp3, wav, aac).
	Src string `json:"src" yaml:"src"`

	// StartFrame is the frame at which this audio track begins.
	StartFrame int `json:"startFrame" yaml:"startFrame"`

	// EndFrame is the frame at which this audio track ends.
	// 0 means play to the end of the track or composition, whichever is shorter.
	EndFrame int `json:"endFrame,omitempty" yaml:"endFrame,omitempty"`

	// Volume is a multiplier from 0.0 (silent) to 1.0 (full). Defaults to 1.0.
	Volume float64 `json:"volume,omitempty" yaml:"volume,omitempty"`

	// Trim trims the audio file before mixing.
	TrimStart time.Duration `json:"trimStart,omitempty" yaml:"trimStart,omitempty"`
	TrimEnd   time.Duration `json:"trimEnd,omitempty" yaml:"trimEnd,omitempty"`
}

// OutputConfig controls the final video output.
type OutputConfig struct {
	// Path is the output file path. e.g. "./output.mp4"
	Path string `json:"path" yaml:"path"`

	// Format is the container format: "mp4", "webm", "gif". Defaults to "mp4".
	Format string `json:"format,omitempty" yaml:"format,omitempty"`

	// VideoCodec overrides the default codec. Defaults to "libx264" for mp4.
	VideoCodec string `json:"videoCodec,omitempty" yaml:"videoCodec,omitempty"`

	// CRF controls quality. Lower = better quality, larger file.
	// Typical range 18–28. Defaults to 20.
	CRF int `json:"crf,omitempty" yaml:"crf,omitempty"`

	// Preset is the ffmpeg encoding preset. Defaults to "medium".
	Preset string `json:"preset,omitempty" yaml:"preset,omitempty"`

	// PixelFormat defaults to "yuv420p" for broad compatibility.
	PixelFormat string `json:"pixelFormat,omitempty" yaml:"pixelFormat,omitempty"`

	// UpscaleWidth/UpscaleHeight optionally upscale final video output.
	// If both are >0 and differ from render dimensions, gorender upscales after frame render.
	UpscaleWidth  int `json:"upscaleWidth,omitempty" yaml:"upscaleWidth,omitempty"`
	UpscaleHeight int `json:"upscaleHeight,omitempty" yaml:"upscaleHeight,omitempty"`
}

// Duration returns the total duration of the composition.
func (c *Composition) Duration() time.Duration {
	if c.FPS == 0 {
		return 0
	}
	return time.Duration(c.DurationFrames) * time.Second / time.Duration(c.FPS)
}

// Defaults fills in zero values with sensible defaults.
func (c *Composition) Defaults() {
	if c.SeekParam == "" {
		c.SeekParam = "frame"
	}
	if c.ReadySignal == "" {
		c.ReadySignal = "window.__READY__ === true"
	}
	if c.ReadyTimeout == 0 {
		c.ReadyTimeout = 5 * time.Second
	}
	if c.FPS == 0 {
		c.FPS = 30
	}
	if c.Width == 0 {
		c.Width = 1080
	}
	if c.Height == 0 {
		c.Height = 1920
	}
	if c.Output.Format == "" {
		c.Output.Format = "mp4"
	}
	if c.Output.CRF == 0 {
		c.Output.CRF = 20
	}
	if c.Output.Preset == "" {
		c.Output.Preset = "medium"
	}
	if c.Output.PixelFormat == "" {
		c.Output.PixelFormat = "yuv420p"
	}
	for i := range c.Audio {
		if c.Audio[i].Volume == 0 {
			c.Audio[i].Volume = 1.0
		}
	}
}
