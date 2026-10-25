package model

import (
	"strings"
	"testing"
)

// resizeBench is a model and the two sizes a benchmark toggles it between.
type resizeBench struct {
	name     string
	from, to [2]int
	output   string
}

// resizeBenches are the models a resize must stay cheap on: many short lines, one chain filling the
// scrollback, and a height change, which reflows nothing.
func resizeBenches() []resizeBench {
	short := strings.Repeat(strings.Repeat("s", 40)+"\r\n", 10000)
	return []resizeBench{
		{name: "short-lines-10k", from: [2]int{120, 40}, to: [2]int{90, 40}, output: short},
		{name: "one-chain-10k", from: [2]int{200, 40}, to: [2]int{80, 40}, output: strings.Repeat("c", 200*10000) + "\r\n"},
		{name: "height-only", from: [2]int{120, 40}, to: [2]int{120, 30}, output: short},
	}
}

func (rb resizeBench) model(b *testing.B) (TerminalModel, Serializer) {
	b.Helper()
	m, ser := New(rb.from[0], rb.from[1], 10000)
	b.Cleanup(m.Close)
	m.Write([]byte(rb.output))
	return m, ser
}

// BenchmarkResize toggles each model between its sizes. Budget: no more time than the keyframe
// the resize forces (BenchmarkSerialize), and no more bytes than the model holds (modelB).
func BenchmarkResize(b *testing.B) {
	for _, rb := range resizeBenches() {
		b.Run(rb.name, func(b *testing.B) {
			m, _ := rb.model(b)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				size := rb.to
				if i%2 == 1 {
					size = rb.from
				}
				m.Resize(size[0], size[1])
			}
			b.ReportMetric(float64(m.ModelBytes()), "modelB")
		})
	}
}

// BenchmarkSerialize is the keyframe each resize forces, over the same models.
func BenchmarkSerialize(b *testing.B) {
	for _, rb := range resizeBenches() {
		b.Run(rb.name, func(b *testing.B) {
			m, ser := rb.model(b)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = ser.Serialize(m)
			}
		})
	}
}
