package model

import (
	"fmt"
	"testing"
)

// BenchmarkDiffEcho: single-keystroke delta (the interactive hot path).
func BenchmarkDiffEcho(b *testing.B) {
	m, _ := New(170, 50, 1000)
	defer m.Close()
	e := NewDiffEmitter()
	e.Prime(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Write([]byte("x"))
		_, _ = e.Emit(m)
	}
}

// BenchmarkDiffFullRepaint: a TUI rewriting the whole screen per frame.
func BenchmarkDiffFullRepaint(b *testing.B) {
	m, _ := New(170, 50, 1000)
	defer m.Close()
	e := NewDiffEmitter()
	e.Prime(m)
	frameA := buildFullScreen('A', 170, 50)
	frameB := buildFullScreen('B', 170, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			m.Write(frameA)
		} else {
			m.Write(frameB)
		}
		_, _ = e.Emit(m)
	}
}

// BenchmarkDiffScrollBurst: cat-style append of 100 lines per emit tick.
func BenchmarkDiffScrollBurst(b *testing.B) {
	m, _ := New(170, 50, 10000)
	defer m.Close()
	e := NewDiffEmitter()
	e.Prime(m)
	var burst []byte
	for i := 0; i < 100; i++ {
		burst = append(burst, []byte(fmt.Sprintf("line %04d with some payload text\r\n", i))...)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Write(burst)
		_, _ = e.Emit(m)
	}
}

func buildFullScreen(ch byte, cols, rows int) []byte {
	out := []byte("\x1b[1;1H")
	line := make([]byte, cols)
	for i := range line {
		line[i] = ch
	}
	for y := 0; y < rows; y++ {
		out = append(out, []byte(fmt.Sprintf("\x1b[%d;1H", y+1))...)
		out = append(out, line...)
	}
	return out
}
