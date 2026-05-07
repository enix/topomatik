package config

import (
	"path/filepath"
	"testing"
	"time"
)

func fixture(name string) string {
	return filepath.Join("testdata", name)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(fixture("does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing fixture file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	_, err := Load(fixture("invalid.yaml"))
	if err == nil {
		t.Fatal("expected YAML parse error")
	}
}

func TestLoad_MinimalAppliesDefaults(t *testing.T) {
	cfg, err := Load(fixture("minimal.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MinimumReconciliationInterval == 0 {
		t.Error("MinimumReconciliationInterval should be defaulted when zero")
	}
	if got, want := cfg.LabelTemplates["topology/zone"], "{{ .hostname.zone }}"; got != want {
		t.Errorf("LabelTemplates[topology/zone]: got %q, want %q", got, want)
	}
}

func TestLoad_FullConfigDecodesAllFields(t *testing.T) {
	cfg, err := Load(fixture("full.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.MinimumReconciliationInterval != 30*time.Second {
		t.Errorf("MinimumReconciliationInterval: got %v, want 30s", cfg.MinimumReconciliationInterval)
	}

	if !cfg.Hardware.Enabled {
		t.Error("Hardware should be enabled")
	}
	if cfg.Hardware.Config.Interval != 10*time.Second {
		t.Errorf("Hardware.Interval: got %v, want 10s", cfg.Hardware.Config.Interval)
	}

	if !cfg.Hostname.Enabled {
		t.Error("Hostname should be enabled")
	}
	if cfg.Hostname.Config.Interval != 5*time.Second {
		t.Errorf("Hostname.Interval: got %v, want 5s", cfg.Hostname.Config.Interval)
	}
	if cfg.Hostname.Config.Pattern == nil {
		t.Fatal("Hostname.Pattern: nil")
	}
	if got, want := cfg.Hostname.Config.Pattern.String(), "(?P<zone>[a-z]+)-(?P<rack>[a-z]+)-(?P<node>[0-9]+)$"; got != want {
		t.Errorf("Hostname.Pattern: got %q, want %q", got, want)
	}

	if !cfg.Network.Enabled {
		t.Error("Network should be enabled")
	}

	tt, ok := cfg.TaintTemplates["specialized"]
	if !ok {
		t.Fatal("TaintTemplates[specialized]: missing")
	}
	if tt.Value != "{{ .hostname.zone }}" || tt.Effect != "NoSchedule" {
		t.Errorf("TaintTemplates[specialized]: got %+v", tt)
	}
}

func TestLoad_RequiredIntervalForEnabledEngines(t *testing.T) {
	cases := []string{
		"hardware-missing-interval.yaml",
		"hostname-missing-interval.yaml",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(fixture(name))
			if err == nil {
				t.Fatal("expected validation error for missing interval")
			}
		})
	}
}

func TestLoad_DisabledEnginesSkipValidation(t *testing.T) {
	cfg, err := Load(fixture("engines-disabled.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Hardware.Enabled || cfg.Hostname.Enabled || cfg.Network.Enabled {
		t.Errorf("expected all engines disabled, got hardware=%v hostname=%v network=%v",
			cfg.Hardware.Enabled, cfg.Hostname.Enabled, cfg.Network.Enabled)
	}
}

func TestLoad_FilesValidation(t *testing.T) {
	cases := []struct {
		name      string
		fixture   string
		wantError bool
	}{
		{"local path resolves", "files-local.yaml", false},
		{"remote URL with interval", "files-remote-with-interval.yaml", false},
		{"remote URL without interval", "files-remote-no-interval.yaml", true},
		{"path that is neither a file nor a URL", "files-bad-path.yaml", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(fixture(tc.fixture))
			if tc.wantError && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
