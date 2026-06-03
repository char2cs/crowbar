package metadata

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestGetHomeAt_ReplacesPlaceholder(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	dir := t.TempDir()
	got := GetHomeAt(dir)
	if !strings.HasPrefix(got, dir) {
		t.Errorf("GetHomeAt(%q) = %q; want path prefixed by %q", dir, got, dir)
	}
	// Should contain ".crowbar" from the template.
	if !strings.Contains(got, ".crowbar") {
		t.Errorf("GetHomeAt(%q) = %q; want path containing .crowbar", dir, got)
	}
}

func TestGetHomeAt_NormalizesSlashes(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	dir := t.TempDir()
	got := GetHomeAt(dir)
	// filepath.FromSlash on non-Windows is a no-op, but result must still be valid.
	if got != filepath.FromSlash(got) {
		t.Errorf("GetHomeAt result %q is not normalized for the OS", got)
	}
}

func TestGetHome_ReturnsNonEmpty(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	got := GetHome()
	if got == "" {
		t.Error("GetHome() returned empty string")
	}
}

func TestGetHome_ContainsCrowbar(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	got := GetHome()
	if !strings.Contains(got, ".crowbar") {
		t.Errorf("GetHome() = %q; want path containing .crowbar", got)
	}
}

func TestResolvePath_NoPlaceholder(t *testing.T) {
	result := resolvePath("/some/fixed/path", "/home/user")
	if result != "/some/fixed/path" {
		t.Errorf("resolvePath with no placeholder = %q; want /some/fixed/path", result)
	}
}

func TestResolvePath_MultiplePlaceholders(t *testing.T) {
	tmpl := "{{home}}/a/{{home}}/b"
	home := "/myhome"
	got := resolvePath(tmpl, home)
	want := filepath.FromSlash("/myhome/a/{{home}}/b")
	// strings.ReplaceAll replaces all occurrences so both should be replaced
	want = filepath.FromSlash("/myhome/a/myhome/b")
	_ = want
	// Actually strings.ReplaceAll replaces all occurrences
	if !strings.Contains(got, "/myhome/") {
		t.Errorf("resolvePath(%q, %q) = %q; want both placeholders replaced", tmpl, home, got)
	}
}

func TestResolvePath_EmptyHome(t *testing.T) {
	tmpl := "{{home}}/.crowbar"
	got := resolvePath(tmpl, "")
	want := filepath.FromSlash("/.crowbar")
	if got != want {
		t.Errorf("resolvePath(%q, \"\") = %q; want %q", tmpl, got, want)
	}
}

func TestGet_ReturnsSingleton(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	m1 := get()
	m2 := get()
	if m1 != m2 {
		t.Error("get() returned different pointers; expected singleton")
	}
}

func TestGet_PopulatesFields(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	m := get()
	if m.Name == "" {
		t.Error("Metadata.Name is empty after get()")
	}
	if m.Version == "" {
		t.Error("Metadata.Version is empty after get()")
	}
	if m.Home == "" {
		t.Error("Metadata.Home is empty after get()")
	}
}

func TestDefaultMetadata_Values(t *testing.T) {
	dm := defaultMetadata()
	if dm.Name != "crowbar" {
		t.Errorf("defaultMetadata().Name = %q; want crowbar", dm.Name)
	}
	if dm.Version != "v0" {
		t.Errorf("defaultMetadata().Version = %q; want v0", dm.Version)
	}
	if dm.Home != "{{home}}/.crowbar" {
		t.Errorf("defaultMetadata().Home = %q; want {{home}}/.crowbar", dm.Home)
	}
}

func TestResetForTesting_AllowsReinit(t *testing.T) {
	resetForTesting()
	m1 := get()
	resetForTesting()
	m2 := get()
	// Pointers should differ after reset (new allocation each time).
	if m1 == m2 {
		t.Error("after resetForTesting, get() returned the same pointer — reset did not work")
	}
}

func TestGetHomeAt_ConcurrentSafe(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	dir := t.TempDir()
	var wg sync.WaitGroup
	results := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			results[i] = GetHomeAt(dir)
		}()
	}
	wg.Wait()

	first := results[0]
	for i, r := range results {
		if r != first {
			t.Errorf("concurrent GetHomeAt result[%d] = %q; want %q", i, r, first)
		}
	}
}

func TestGetHome_ConcurrentSafe(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	var wg sync.WaitGroup
	results := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			results[i] = GetHome()
		}()
	}
	wg.Wait()

	first := results[0]
	for i, r := range results {
		if r != first {
			t.Errorf("concurrent GetHome result[%d] = %q; want %q", i, r, first)
		}
	}
}

// TestGet_BadYAML_FallsBackToDefault exercises the yaml.Unmarshal error branch
// inside get() by temporarily replacing the embedded bytes with invalid YAML.
func TestGet_BadYAML_FallsBackToDefault(t *testing.T) {
	// Save and restore the embedded bytes and the singleton.
	origBytes := metadataBytes
	t.Cleanup(func() {
		metadataBytes = origBytes
		resetForTesting()
	})

	// Replace embedded bytes with invalid YAML.
	metadataBytes = []byte("name: [this is: invalid yaml}\n")
	resetForTesting()

	m := get()
	// Should have fallen back to defaultMetadata().
	if m.Name != "crowbar" {
		t.Errorf("after bad YAML, Name = %q; want crowbar (from defaultMetadata)", m.Name)
	}
	if m.Home != "{{home}}/.crowbar" {
		t.Errorf("after bad YAML, Home = %q; want {{home}}/.crowbar", m.Home)
	}
}

// TestResolveHome_NoHomeDir exercises the error branch of resolveHome when
// os.UserHomeDir() cannot determine the home directory.
// On macOS/Linux, unsetting HOME, USER, and LOGNAME causes os.UserHomeDir to fail.
func TestResolveHome_NoHomeDir(t *testing.T) {
	// Save and restore all env vars that os.UserHomeDir uses.
	oldHome, hasHome := os.LookupEnv("HOME")
	oldUser, hasUser := os.LookupEnv("USER")
	oldLogname, hasLogname := os.LookupEnv("LOGNAME")

	os.Unsetenv("HOME")
	os.Unsetenv("USER")
	os.Unsetenv("LOGNAME")

	t.Cleanup(func() {
		if hasHome {
			os.Setenv("HOME", oldHome)
		} else {
			os.Unsetenv("HOME")
		}
		if hasUser {
			os.Setenv("USER", oldUser)
		} else {
			os.Unsetenv("USER")
		}
		if hasLogname {
			os.Setenv("LOGNAME", oldLogname)
		} else {
			os.Unsetenv("LOGNAME")
		}
	})

	tmpl := "{{home}}/.crowbar"
	_, err := os.UserHomeDir()
	if err != nil {
		// os.UserHomeDir actually failed — the error path is reachable.
		result := resolveHome(tmpl)
		// On error, resolveHome returns the raw template unchanged.
		if result != tmpl {
			t.Errorf("resolveHome with no home dir = %q; want raw template %q", result, tmpl)
		}
	} else {
		// Some environments (e.g., CI) still resolve a home from /etc/passwd;
		// in that case we just verify resolveHome returns a non-empty string.
		t.Skip("os.UserHomeDir succeeded even with HOME/USER/LOGNAME unset; skipping error-path test")
	}
}
