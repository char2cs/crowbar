package config

import (
	"os"
	"path/filepath"
	"testing"
)

// overrideHome temporarily redirects metadata.GetHome() to a temp directory
// by writing a user config file at the expected path. Since config.Get() uses
// metadata.GetHome() internally, we patch HOME so the process resolves the
// home correctly for isolation.
func patchHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func resetConfig(t *testing.T) {
	t.Helper()
	resetForTesting()
}

func TestGet_ReturnsDefaults_WhenNoUserFile(t *testing.T) {
	patchHome(t)
	resetConfig(t)
	defer resetForTesting()

	cfg := Get()
	if cfg.Intelligence.Low == "" {
		t.Error("Intelligence.Low should not be empty (from embedded defaults)")
	}
	if cfg.Intelligence.Medium == "" {
		t.Error("Intelligence.Medium should not be empty (from embedded defaults)")
	}
	if cfg.Intelligence.High == "" {
		t.Error("Intelligence.High should not be empty (from embedded defaults)")
	}
	if cfg.API.Host == "" {
		t.Error("API.Host should not be empty (from embedded defaults)")
	}
}

func TestGet_OverlaysUserFile(t *testing.T) {
	dir := patchHome(t)
	resetConfig(t)
	defer resetForTesting()

	// Create the crowbar home dir and write a user config.
	crowbarHome := filepath.Join(dir, ".crowbar")
	if err := os.MkdirAll(crowbarHome, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	userConfig := []byte("intelligence:\n  low: custom-model\n")
	if err := os.WriteFile(filepath.Join(crowbarHome, "config.yaml"), userConfig, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := Get()
	if cfg.Intelligence.Low != "custom-model" {
		t.Errorf("Intelligence.Low = %q; want custom-model", cfg.Intelligence.Low)
	}
	// Fields not in user file keep embedded defaults.
	if cfg.Intelligence.Medium == "" {
		t.Error("Intelligence.Medium should retain embedded default")
	}
}

func TestGet_Singleton(t *testing.T) {
	patchHome(t)
	resetConfig(t)
	defer resetForTesting()

	c1 := Get()
	c2 := Get()
	if c1.Intelligence.Low != c2.Intelligence.Low {
		t.Error("Get() is not returning the same singleton value on repeated calls")
	}
}

func TestGet_InvalidUserYAML_FallsBackToDefaults(t *testing.T) {
	dir := patchHome(t)
	resetConfig(t)
	defer resetForTesting()

	crowbarHome := filepath.Join(dir, ".crowbar")
	if err := os.MkdirAll(crowbarHome, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Write deliberately broken YAML.
	badYAML := []byte("intelligence: [this is not: valid yaml}\n")
	if err := os.WriteFile(filepath.Join(crowbarHome, "config.yaml"), badYAML, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := Get()
	// On YAML parse failure, getDefault() is called again; embedded defaults are used.
	if cfg.Intelligence.Low == "" {
		t.Error("after bad YAML, Intelligence.Low should be non-empty (embedded defaults)")
	}
}

func TestGetIntelligence_ReturnsIntelligenceConfig(t *testing.T) {
	patchHome(t)
	resetConfig(t)
	defer resetForTesting()

	intel := GetIntelligence()
	full := Get().Intelligence
	if intel.Low != full.Low || intel.Medium != full.Medium || intel.High != full.High {
		t.Errorf("GetIntelligence() = %+v; want %+v", intel, full)
	}
}

func TestGetAPI_ReturnsAPIConfig(t *testing.T) {
	patchHome(t)
	resetConfig(t)
	defer resetForTesting()

	api := GetAPI()
	full := Get().API
	if api.Host != full.Host {
		t.Errorf("GetAPI().Host = %q; want %q", api.Host, full.Host)
	}
}

func TestModelFor_Low(t *testing.T) {
	c := IntelligenceConfig{Low: "low-model", Medium: "med-model", High: "high-model"}
	if got := c.ModelFor("low"); got != "low-model" {
		t.Errorf("ModelFor(\"low\") = %q; want low-model", got)
	}
}

func TestModelFor_Medium(t *testing.T) {
	c := IntelligenceConfig{Low: "low-model", Medium: "med-model", High: "high-model"}
	if got := c.ModelFor("medium"); got != "med-model" {
		t.Errorf("ModelFor(\"medium\") = %q; want med-model", got)
	}
}

func TestModelFor_High(t *testing.T) {
	c := IntelligenceConfig{Low: "low-model", Medium: "med-model", High: "high-model"}
	if got := c.ModelFor("high"); got != "high-model" {
		t.Errorf("ModelFor(\"high\") = %q; want high-model", got)
	}
}

func TestModelFor_Unknown(t *testing.T) {
	c := IntelligenceConfig{Low: "low-model", Medium: "med-model", High: "high-model"}
	if got := c.ModelFor("unknown"); got != "" {
		t.Errorf("ModelFor(\"unknown\") = %q; want empty string", got)
	}
}

func TestModelFor_CaseInsensitive(t *testing.T) {
	c := IntelligenceConfig{Low: "low-model", Medium: "med-model", High: "high-model"}
	cases := []struct {
		level string
		want  string
	}{
		{"LOW", "low-model"},
		{"MEDIUM", "med-model"},
		{"HIGH", "high-model"},
		{"Low", "low-model"},
		{"Medium", "med-model"},
		{"High", "high-model"},
	}
	for _, tc := range cases {
		if got := c.ModelFor(tc.level); got != tc.want {
			t.Errorf("ModelFor(%q) = %q; want %q", tc.level, got, tc.want)
		}
	}
}

func TestModelFor_EmptyLevel(t *testing.T) {
	c := IntelligenceConfig{Low: "low-model", Medium: "med-model", High: "high-model"}
	if got := c.ModelFor(""); got != "" {
		t.Errorf("ModelFor(\"\") = %q; want empty string", got)
	}
}

func TestGetDefault_PopulatesAllFields(t *testing.T) {
	c := getDefault()
	if c.Intelligence.Low == "" {
		t.Error("getDefault().Intelligence.Low is empty")
	}
	if c.Intelligence.Medium == "" {
		t.Error("getDefault().Intelligence.Medium is empty")
	}
	if c.Intelligence.High == "" {
		t.Error("getDefault().Intelligence.High is empty")
	}
	if c.API.Host == "" {
		t.Error("getDefault().API.Host is empty")
	}
}

func TestResetForTesting_AllowsReinit(t *testing.T) {
	patchHome(t)
	resetForTesting()
	c1 := Get()
	resetForTesting()
	c2 := Get()
	// Both should have equal contents (same embedded defaults); this just
	// verifies reset + re-init cycle does not panic.
	if c1.Intelligence.Low != c2.Intelligence.Low {
		t.Errorf("after reset, Intelligence.Low differs: %q vs %q", c1.Intelligence.Low, c2.Intelligence.Low)
	}
}

// TestGetDefault_PanicsOnBadEmbeddedYAML exercises the panic branch of getDefault()
// by temporarily replacing defaultConfigBytes with invalid YAML.
func TestGetDefault_PanicsOnBadEmbeddedYAML(t *testing.T) {
	origBytes := defaultConfigBytes
	t.Cleanup(func() {
		defaultConfigBytes = origBytes
	})

	defaultConfigBytes = []byte("intelligence: [this is: not valid yaml}\n")

	defer func() {
		r := recover()
		if r == nil {
			t.Error("getDefault() should have panicked with invalid embedded YAML")
		}
	}()

	// This should panic.
	getDefault()
}
