package presets

import "strings"

type Name string

const (
	Final              Name = "final"
	Fast               Name = "fast"
	ParityStrict       Name = "parity-strict"
	SpeedBalanced      Name = "speed-balanced"
	SpeedMax           Name = "speed-max"
	ProductionBalanced Name = "production-balanced"
	ProductionFast     Name = "production-fast"
	Preview            Name = "preview"
	Draft              Name = "draft"
	LowBandwidth       Name = "low-bandwidth"
	CPUConstrained     Name = "cpu-constrained"
	DeterministicCI    Name = "deterministic-ci"
	DebugTrace         Name = "debug-trace"
)

type Config struct {
	ExperimentalPipeline bool
	CaptureFormat        string
	CaptureJPEGQuality   int
	EncoderPreset        string
	CRF                  int
	DefaultWidth         int
	DefaultHeight        int
	DefaultUpscaleWidth  int
	DefaultUpscaleHeight int
}

var configs = map[Name]Config{
	Final: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   90,
		EncoderPreset:        "veryfast",
		CRF:                  21,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	Fast: {
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   82,
		EncoderPreset:        "veryfast",
		CRF:                  24,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	ParityStrict: {
		CaptureFormat:        "png",
		EncoderPreset:        "medium",
		CRF:                  18,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	SpeedBalanced: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   90,
		EncoderPreset:        "veryfast",
		CRF:                  21,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	SpeedMax: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   88,
		EncoderPreset:        "ultrafast",
		CRF:                  24,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	ProductionBalanced: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   90,
		EncoderPreset:        "veryfast",
		CRF:                  21,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	ProductionFast: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   86,
		EncoderPreset:        "superfast",
		CRF:                  23,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	Preview: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   80,
		EncoderPreset:        "ultrafast",
		CRF:                  28,
		DefaultWidth:         540,
		DefaultHeight:        960,
	},
	Draft: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   82,
		EncoderPreset:        "ultrafast",
		CRF:                  28,
		DefaultWidth:         720,
		DefaultHeight:        1280,
	},
	LowBandwidth: {
		ExperimentalPipeline: true,
		CaptureFormat:        "jpeg",
		CaptureJPEGQuality:   82,
		EncoderPreset:        "veryfast",
		CRF:                  28,
		DefaultWidth:         720,
		DefaultHeight:        1280,
	},
	CPUConstrained: {
		CaptureFormat:      "jpeg",
		CaptureJPEGQuality: 82,
		EncoderPreset:      "veryfast",
		CRF:                24,
		DefaultWidth:       720,
		DefaultHeight:      1280,
	},
	DeterministicCI: {
		CaptureFormat:        "png",
		EncoderPreset:        "medium",
		CRF:                  20,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
	DebugTrace: {
		CaptureFormat:        "png",
		EncoderPreset:        "medium",
		CRF:                  20,
		DefaultWidth:         720,
		DefaultHeight:        1280,
		DefaultUpscaleWidth:  1080,
		DefaultUpscaleHeight: 1920,
	},
}

func AliasedProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "final":
		return string(Final)
	case "fast":
		return string(Fast)
	default:
		return ""
	}
}

func Resolve(name string) (Config, bool) {
	n := Name(strings.ToLower(strings.TrimSpace(name)))
	cfg, ok := configs[n]
	return cfg, ok
}
