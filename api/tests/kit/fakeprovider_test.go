//go:build integration

package kit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFakeProvider_WithoutAComposerTheFakeCLIEchoesInput(t *testing.T) {
	p := startFake(t)

	p.write("typed text\n")

	p.await("echo", func(s string) bool { return strings.Contains(s, "typed text") })
	require.NotContains(t, p.screen(), bracketedPasteOn)
}

func TestInstallFakeProvider_ComposerKnobsPassDescriptorValidation(t *testing.T) {
	home := t.TempDir()

	InstallFakeProvider(t, home, FakeSpec{Base: "claude", ID: "composer", Composer: &Composer{Prefix: "❯ ", Draft: "d", Ghost: "g", Busy: true}})

	raw, err := os.ReadFile(filepath.Join(home, "descriptors", "composer.yaml"))
	require.NoError(t, err)
	for _, name := range []string{"FAKECLI_COMPOSER", "FAKECLI_COMPOSER_PREFIX", "FAKECLI_COMPOSER_DRAFT", "FAKECLI_COMPOSER_GHOST", "FAKECLI_COMPOSER_BUSY"} {
		require.Contains(t, string(raw), name)
	}
}
