package payload_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
)

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func TestString_ReadsNestedAndDegradesToEmpty(t *testing.T) {
	p := decode(t, `{"a":{"b":"deep"},"top":"v","n":7,"null":null,"arr":[1]}`)

	testCases := []struct {
		name string
		path string
		want string
	}{
		{"bare key", "top", "v"},
		{"dotted descent", "a.b", "deep"},
		{"empty path", "", ""},
		{"missing key", "nope", ""},
		{"missing nested key", "a.nope", ""},
		{"descend through a scalar", "top.b", ""},
		{"non-string leaf", "n", ""},
		{"explicit json null", "null", ""},
		{"array leaf", "arr", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, payload.String(p, tc.path))
		})
	}
}

func TestCount_LengthOfArrayLeafOnly(t *testing.T) {
	p := decode(t, `{"tasks":[1,2,3],"empty":[],"obj":{},"scalar":4}`)

	testCases := []struct {
		name string
		path string
		want int
	}{
		{"three entries", "tasks", 3},
		{"empty array", "empty", 0},
		{"object is not an array", "obj", 0},
		{"scalar is not an array", "scalar", 0},
		{"missing path", "gone", 0},
		{"empty path", "", 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, payload.Count(p, tc.path))
		})
	}
}

func TestFloat_AcceptsNumbersAndRejectsNumericStrings(t *testing.T) {
	p := decode(t, `{"pct":19.5,"whole":200000,"quoted":"19","nil":null}`)

	got, ok := payload.Float(p, "pct")
	require.True(t, ok)
	assert.InDelta(t, 19.5, got, 0.0001)

	whole, ok := payload.Int(p, "whole")
	require.True(t, ok)
	assert.Equal(t, 200000, whole)

	_, ok = payload.Float(p, "quoted")
	assert.False(t, ok, "a quoted number is a changed contract, not a number")

	_, ok = payload.Float(p, "nil")
	assert.False(t, ok)

	_, ok = payload.Int(p, "missing")
	assert.False(t, ok)
}

func TestFloat_AcceptsJSONNumberAndGoNumerics(t *testing.T) {
	p := map[string]any{
		"jsonNumber": json.Number("42.5"),
		"bad":        json.Number("not-a-number"),
		"int":        7,
		"int64":      int64(8),
		"float32":    float32(1.5),
	}

	for _, tc := range []struct {
		name string
		path string
		want float64
	}{
		{"json.Number", "jsonNumber", 42.5},
		{"int", "int", 7},
		{"int64", "int64", 8},
		{"float32", "float32", 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := payload.Float(p, tc.path)
			require.True(t, ok)
			assert.InDelta(t, tc.want, got, 0.0001)
		})
	}

	_, ok := payload.Float(p, "bad")
	assert.False(t, ok)
}

func TestBool_ReadsBooleanLeafOnly(t *testing.T) {
	p := decode(t, `{"yes":true,"no":false,"str":"true"}`)

	v, ok := payload.Bool(p, "yes")
	require.True(t, ok)
	assert.True(t, v)

	v, ok = payload.Bool(p, "no")
	require.True(t, ok)
	assert.False(t, v)

	_, ok = payload.Bool(p, "str")
	assert.False(t, ok)

	_, ok = payload.Bool(p, "missing")
	assert.False(t, ok)
}

func TestTime_ParsesRFC3339AndTreatsGarbageAsAbsent(t *testing.T) {
	p := decode(t, `{"at":"2026-08-17T10:11:12Z","junk":"soon","empty":""}`)

	got, ok := payload.Time(p, "at")
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, 8, 17, 10, 11, 12, 0, time.UTC), got.UTC())

	_, ok = payload.Time(p, "junk")
	assert.False(t, ok, "an unparseable reset time must be absent, not zero")

	_, ok = payload.Time(p, "empty")
	assert.False(t, ok)

	_, ok = payload.Time(p, "missing")
	assert.False(t, ok)
}

func TestJSON_ReencodesSubtreesAndPassesStringsThrough(t *testing.T) {
	p := decode(t, `{"input":{"file":"a.go","n":2},"text":"plain result","blank":"","nil":null,"arr":[1,2]}`)

	assert.JSONEq(t, `{"file":"a.go","n":2}`, string(payload.JSON(p, "input")))
	assert.Equal(t, "plain result", string(payload.JSON(p, "text")),
		"a plain-text result must not be stored as an escaped JSON string")
	assert.JSONEq(t, `[1,2]`, string(payload.JSON(p, "arr")))
	assert.Nil(t, payload.JSON(p, "blank"))
	assert.Nil(t, payload.JSON(p, "nil"))
	assert.Nil(t, payload.JSON(p, "missing"))
	assert.Nil(t, payload.JSON(p, ""))
}

func TestObjects_ReadsAnArrayOfObjects(t *testing.T) {
	p := decode(t, `{"tool_input":{"questions":[{"header":"a"},{"header":"b"}]}}`)

	got := payload.Objects(p, "tool_input.questions")

	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0]["header"])
	assert.Equal(t, "b", got[1]["header"])
}

// A provider that starts mixing scalars into a list of objects must cost that
// entry, not the whole list.
func TestObjects_SkipsNonObjectElements(t *testing.T) {
	p := decode(t, `{"items":[{"k":1},"not an object",null,{"k":2}]}`)

	assert.Len(t, payload.Objects(p, "items"), 2)
}

func TestObjects_AbsentAndWrongShapesYieldNothing(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{name: "missing", path: "nope"},
		{name: "not an array", path: "scalar"},
		{name: "empty path", path: ""},
		{name: "through a scalar", path: "scalar.deeper"},
	}
	p := decode(t, `{"scalar":7}`)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Nil(t, payload.Objects(p, tc.path))
		})
	}
}

func TestObject_ReturnsTheSubtreeAsAMap(t *testing.T) {
	p := decode(t, `{"tool_input":{"command":"ls","count":2}}`)
	got := payload.Object(p, "tool_input")
	require.NotNil(t, got)
	assert.Equal(t, "ls", got["command"])
}

func TestObject_AbsentAndWrongShapesYieldNil(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{name: "missing", path: "nope"},
		{name: "not an object", path: "scalar"},
		{name: "an array is not an object", path: "list"},
		{name: "empty path", path: ""},
	}
	p := decode(t, `{"scalar":7,"list":[1,2]}`)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Nil(t, payload.Object(p, tc.path))
		})
	}
}
