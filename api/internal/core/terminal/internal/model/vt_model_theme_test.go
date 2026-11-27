package model

import (
	"image/color"
	"strings"
	"testing"
)

var (
	white = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	black = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
)

func queryReportedBackground(t *testing.T, m *vtModel) string {
	t.Helper()
	got := make(chan []byte, 4)
	m.SetResponseSink(func(p []byte) {
		got <- append([]byte(nil), p...)
	})
	m.Write([]byte("\x1b]11;?\x07"))
	return string(<-got)
}

func TestSetDefaultColors_ReportedViaOSC11Query(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(white, black)

	reply := queryReportedBackground(t, m)
	if !strings.Contains(reply, "\x1b]11;rgb:ffff/ffff/ffff") {
		t.Fatalf("OSC 11 reply = %q, want a white (ffff/ffff/ffff) background", reply)
	}
}

func TestSetDefaultColors_DarkReportedViaOSC11Query(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(black, white)

	reply := queryReportedBackground(t, m)
	if !strings.Contains(reply, "\x1b]11;rgb:0000/0000/0000") {
		t.Fatalf("OSC 11 reply = %q, want a black (0000/0000/0000) background", reply)
	}
}

func TestSetDefaultColors_NilLeavesReportUnchanged(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.SetDefaultColors(white, black)
	m.SetDefaultColors(nil, nil)

	reply := queryReportedBackground(t, m)
	if !strings.Contains(reply, "\x1b]11;rgb:ffff/ffff/ffff") {
		t.Fatalf("OSC 11 reply = %q, want the prior white background preserved", reply)
	}
}

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

func TestThemeNotify_SurvivesRecreateEmu(t *testing.T) {
	m := newVTModel(20, 5, 100)
	t.Cleanup(func() { m.Close() })

	m.Write([]byte("\x1b[?2031h"))
	m.recreateEmu(m.emu.Width(), m.emu.Height())
	if !m.ThemeNotifyEnabled() {
		t.Fatal("theme-notify subscription lost across parse-panic recreate")
	}
}
