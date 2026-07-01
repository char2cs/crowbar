package model

import "testing"

func TestSetCharsetDesignatesSlots(t *testing.T) {
	s := newShadowState()
	s.setCharset(0, '0')
	s.setCharset(1, 'A')
	if s.g0 != '0' {
		t.Fatalf("g0 = %q, want '0'", s.g0)
	}
	if s.g1 != 'A' {
		t.Fatalf("g1 = %q, want 'A'", s.g1)
	}
}

func TestSetModeIgnoresUntrackedModes(t *testing.T) {
	s := newShadowState()
	s.setMode(9999, true)
	if _, ok := s.modes[9999]; ok {
		t.Fatal("untracked mode was retained")
	}
	s.setMode(1000, true)
	if !s.modes[1000] {
		t.Fatal("tracked mode 1000 not retained")
	}
}
