package model

import (
	"image/color"
	"strings"
	"testing"
	"time"
)

var (
	white = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	black = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
)

// queryReportedBackground installs a response sink, sends an OSC 11 background-color
// QUERY, and returns the raw reply the emulator hands back — the exact bytes a TUI
// (Claude Code's `auto` theme) reads to detect a light vs dark terminal.
func queryReportedBackground(t *testing.T, m *vtModel) string {
	t.Helper()
	got := make(chan []byte, 4)
	m.SetResponseSink(func(p []byte) {
		got <- append([]byte(nil), p...)
	})
	m.Write([]byte("\x1b]11;?\x07"))
	select {
	case reply := <-got:
		return string(reply)
	case <-time.After(2 * time.Second):
		t.Fatal("no OSC 11 reply reached the sink")
		return ""
	}
}

// TestSetDefaultColors_ReportedViaOSC11Query is the core of Gap A: after the daemon is
// told the theme's terminal background, an OSC 11 query must answer with that colour
// (not x/vt's hardcoded default black), so a querying TUI detects the right polarity.
func TestSetDefaultColors_ReportedViaOSC11Query(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(white, black)

	reply := queryReportedBackground(t, m)
	if !strings.Contains(reply, "\x1b]11;rgb:ffff/ffff/ffff") {
		t.Fatalf("OSC 11 reply = %q, want a white (ffff/ffff/ffff) background", reply)
	}
}

// TestSetDefaultColors_DarkReportedViaOSC11Query is the mirror: a dark theme reports a
// dark background.
func TestSetDefaultColors_DarkReportedViaOSC11Query(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(black, white)

	reply := queryReportedBackground(t, m)
	if !strings.Contains(reply, "\x1b]11;rgb:0000/0000/0000") {
		t.Fatalf("OSC 11 reply = %q, want a black (0000/0000/0000) background", reply)
	}
}

// TestSetDefaultColors_NilLeavesReportUnchanged: a nil channel (unparseable colour on the
// wire) must not clobber the other channel or reset the report to black.
func TestSetDefaultColors_NilLeavesReportUnchanged(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(white, black)
	m.SetDefaultColors(nil, nil) // both unparseable — must be a no-op

	reply := queryReportedBackground(t, m)
	if !strings.Contains(reply, "\x1b]11;rgb:ffff/ffff/ffff") {
		t.Fatalf("OSC 11 reply = %q, want the prior white background preserved", reply)
	}
}

// TestSetDefaultColors_NotReEmittedInSerialize is the transparency-leak guard: setting the
// REPORTED default colours must not make the serializer re-emit an OSC 10/11 SET to the
// client xterm. The terminal background is transparent (#00000000) by design; re-emitting an
// opaque OSC 11 would paint over the glass. Only an APP-issued OSC 11 (shadow.bgSet) may be
// re-emitted, and this path never sets that flag.
func TestSetDefaultColors_NotReEmittedInSerialize(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(white, black)

	out := string(vtSerializer{}.Serialize(m))
	if strings.Contains(out, "\x1b]11;") {
		t.Fatalf("serialize re-emitted an OSC 11 default-colour set (transparency leak): %q", out)
	}
	if strings.Contains(out, "\x1b]10;") {
		t.Fatalf("serialize re-emitted an OSC 10 default-colour set: %q", out)
	}
}

// TestSetDefaultColors_SurvivesRecreateEmu: a recovered parse panic rebuilds a blank
// emulator (default bg = black). The theme report must be re-applied so a still-running
// app that re-queries after the app's repaint still sees the correct background.
func TestSetDefaultColors_SurvivesRecreateEmu(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(white, black)
	m.recreateEmu(m.emu.Width(), m.emu.Height())

	reply := queryReportedBackground(t, m)
	if !strings.Contains(reply, "rgb:ffff/ffff/ffff") {
		t.Fatalf("reported background lost across recreateEmu: %q", reply)
	}
}

// TestThemeNotify_TrackedFromMode2031: DEC private mode 2031 is the app's subscription to
// theme-change notifications. The model must track it via the enable/disable mode callbacks.
func TestThemeNotify_TrackedFromMode2031(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	if m.ThemeNotifyEnabled() {
		t.Fatal("theme-notify enabled before any ?2031h")
	}
	m.Write([]byte("\x1b[?2031h"))
	if !m.ThemeNotifyEnabled() {
		t.Fatal("theme-notify not enabled after ?2031h")
	}
	m.Write([]byte("\x1b[?2031l"))
	if m.ThemeNotifyEnabled() {
		t.Fatal("theme-notify still enabled after ?2031l")
	}
}

// TestThemeNotify_ClearedOnForegroundReset: when the foreground app dies and the shell
// returns, its 2031 subscription must lapse — otherwise a later theme switch would inject a
// CSI ?997;n report into the shell's input and corrupt the prompt.
func TestThemeNotify_ClearedOnForegroundReset(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.Write([]byte("\x1b[?2031h"))
	if !m.ThemeNotifyEnabled() {
		t.Fatal("precondition: 2031 should be enabled")
	}
	m.OnForegroundReset()
	if m.ThemeNotifyEnabled() {
		t.Fatal("theme-notify not cleared on foreground reset (app death)")
	}
}

// TestThemeNotify_SurvivesRecreateEmu: a parse panic is not an app exit — the app is still
// running and still subscribed. The subscription must survive the emulator recreate (it is
// model state, not emulator/shadow parser state).
func TestThemeNotify_SurvivesRecreateEmu(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.Write([]byte("\x1b[?2031h"))
	m.recreateEmu(m.emu.Width(), m.emu.Height())
	if !m.ThemeNotifyEnabled() {
		t.Fatal("theme-notify subscription lost across parse-panic recreate")
	}
}
