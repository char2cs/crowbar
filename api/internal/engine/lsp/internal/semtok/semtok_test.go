package semtok

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestLegend_PinsTheWireContract(t *testing.T) {
	wantTypes := []string{
		"namespace", "type", "class", "enum", "interface", "struct", "typeParameter",
		"parameter", "variable", "property", "enumMember", "event", "function",
		"method", "macro", "keyword", "modifier", "comment", "string", "number",
		"regexp", "operator", "decorator", "label", "tag", "attribute",
	}
	wantMods := []string{
		"readonly", "declaration", "definition", "static", "deprecated", "abstract",
		"async", "modification", "documentation", "defaultLibrary",
	}
	if !reflect.DeepEqual(TokenTypes, wantTypes) {
		t.Fatalf("TokenTypes = %v", TokenTypes)
	}
	if !reflect.DeepEqual(TokenModifiers, wantMods) {
		t.Fatalf("TokenModifiers = %v", TokenModifiers)
	}
}

func caps(t *testing.T, provider string) json.RawMessage {
	t.Helper()
	return json.RawMessage(`{"semanticTokensProvider":` + provider + `}`)
}

func TestFromCapabilities_ReadsTheOptionShapes(t *testing.T) {
	cases := []struct {
		name             string
		provider         string
		full, delta, rng bool
	}{
		{"bools", `{"legend":{"tokenTypes":[]},"full":true,"range":true}`, true, false, true},
		{"delta object", `{"legend":{"tokenTypes":[]},"full":{"delta":true}}`, true, true, false},
		{"empty objects", `{"legend":{"tokenTypes":[]},"full":{},"range":{}}`, true, false, true},
		{"off", `{"legend":{"tokenTypes":[]},"full":false}`, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := FromCapabilities(caps(t, tc.provider))
			if s.Full() != tc.full || s.Delta() != tc.delta || s.Range() != tc.rng {
				t.Fatalf("full=%v delta=%v range=%v", s.Full(), s.Delta(), s.Range())
			}
		})
	}
}

func TestFromCapabilities_AbsentOrMalformedSupportsNothing(t *testing.T) {
	for _, raw := range []string{`{}`, `not json`, ``, `{"semanticTokensProvider":null}`} {
		s := FromCapabilities(json.RawMessage(raw))
		if s.Full() || s.Range() || s.Delta() {
			t.Fatalf("%q: expected no support", raw)
		}
	}
}

func rustLike(t *testing.T) Support {
	t.Helper()
	return FromCapabilities(caps(t, `{
		"legend":{"tokenTypes":["variable","lifetime","unknownThing"],
		          "tokenModifiers":["static","constant","local"]},
		"full":{"delta":true}}`))
}

func TestRemapTokens_RewritesOnlyTypeAndModifierSlots(t *testing.T) {
	s := rustLike(t)
	data := []uint32{
		1, 2, 3, 0, 0b011,
		0, 4, 5, 1, 0b100,
		2, 0, 1, 2, 0,
		0, 1, 1, 9, 0b1000,
	}
	s.remapTokens(data)
	unstyled := uint32(len(TokenTypes))
	want := []uint32{
		1, 2, 3, 8, 1<<3 | 1<<0,
		0, 4, 5, 6, 0,
		2, 0, 1, unstyled, 0,
		0, 1, 1, unstyled, 0,
	}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("got %v\nwant %v", data, want)
	}
}

func TestRemapDelta_CommutesWithApplyingTheEdits(t *testing.T) {
	s := rustLike(t)
	old := []uint32{
		0, 0, 3, 0, 1,
		1, 2, 4, 1, 2,
		1, 0, 2, 0, 0,
	}
	edits := []Edit{{Start: 7, DeleteCount: 4, Data: []uint32{9, 0, 0, 2}}}
	newFull := apply(old, edits)

	oldCanon := append([]uint32(nil), old...)
	s.remapTokens(oldCanon)
	if err := s.remapDelta(edits); err != nil {
		t.Fatal(err)
	}
	got := apply(oldCanon, edits)

	want := append([]uint32(nil), newFull...)
	s.remapTokens(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestRemapDelta_MultipleEditsUseTheShiftedOffset(t *testing.T) {
	s := rustLike(t)
	old := make([]uint32, 20)
	for i := range old {
		old[i] = uint32(i % 2)
	}
	edits := []Edit{
		{Start: 15, DeleteCount: 5, Data: []uint32{1, 1, 1, 1, 1}},
		{Start: 0, DeleteCount: 0, Data: []uint32{0, 0, 1, 0, 1}},
	}
	newFull := apply(old, edits)
	oldCanon := append([]uint32(nil), old...)
	s.remapTokens(oldCanon)
	if err := s.remapDelta(edits); err != nil {
		t.Fatal(err)
	}
	got := apply(oldCanon, edits)
	want := append([]uint32(nil), newFull...)
	s.remapTokens(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestRemapDelta_RejectsEditsThatMoveKeptTokensOffTheirSlots(t *testing.T) {
	s := rustLike(t)
	edits := []Edit{
		{Start: 0, DeleteCount: 0, Data: []uint32{1, 2}},
		{Start: 10, DeleteCount: 2, Data: nil},
	}
	if err := s.remapDelta(edits); !errors.Is(err, ErrMisalignedDelta) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemap_ShapesTheWireResult(t *testing.T) {
	s := rustLike(t)
	cases := map[string]string{
		`null`:                                ``,
		`{"resultId":"7","data":[0,1,2,1,0]}`: `{"resultId":"7","data":[0,1,2,6,0]}`,
		`{"data":null}`:                       `{"data":[]}`,
		`{"resultId":"8","edits":[{"start":3,"deleteCount":1,"data":[1]}]}`: `{"resultId":"8","edits":[{"start":3,"deleteCount":1,"data":[6]}]}`,
	}
	for in, want := range cases {
		got, err := s.Remap(json.RawMessage(in))
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if string(got) != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
}

func apply(old []uint32, edits []Edit) []uint32 {
	sorted := append([]Edit(nil), edits...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Start < sorted[j-1].Start; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	var out []uint32
	pos := uint32(0)
	for _, e := range sorted {
		out = append(out, old[pos:e.Start]...)
		out = append(out, e.Data...)
		pos = e.Start + e.DeleteCount
	}
	return append(out, old[pos:]...)
}
