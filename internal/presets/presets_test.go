package presets

import "testing"

func TestResolve(t *testing.T) {
	cfg, ok := Resolve("speed-balanced")
	if !ok {
		t.Fatalf("expected speed-balanced preset")
	}
	if !cfg.ExperimentalPipeline {
		t.Fatalf("expected experimental pipeline enabled")
	}
	if cfg.CaptureFormat != "jpeg" {
		t.Fatalf("unexpected capture format: %s", cfg.CaptureFormat)
	}
}

func TestAliasedProfile(t *testing.T) {
	if got := AliasedProfile("final"); got != "final" {
		t.Fatalf("unexpected final alias: %s", got)
	}
	if got := AliasedProfile("fast"); got != "fast" {
		t.Fatalf("unexpected fast alias: %s", got)
	}
	if got := AliasedProfile("unknown"); got != "" {
		t.Fatalf("expected empty alias, got %s", got)
	}
}
