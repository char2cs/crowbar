package server

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenDocs_AddAndList(t *testing.T) {
	d := newOpenDocs()
	d.Add("file:///b.go")
	d.Add("file:///a.go")
	d.Add("file:///c.go")

	assert.Equal(t, []string{"file:///a.go", "file:///b.go", "file:///c.go"}, d.List())
}

func TestOpenDocs_AddIdempotent(t *testing.T) {
	d := newOpenDocs()
	d.Add("file:///a.go")
	d.Add("file:///a.go")

	assert.Equal(t, []string{"file:///a.go"}, d.List())
}

func TestOpenDocs_Remove(t *testing.T) {
	d := newOpenDocs()
	d.Add("file:///a.go")
	d.Add("file:///b.go")
	d.Remove("file:///a.go")

	assert.Equal(t, []string{"file:///b.go"}, d.List())
}

func TestOpenDocs_RemoveMissingIsNoop(t *testing.T) {
	d := newOpenDocs()
	d.Add("file:///a.go")
	d.Remove("file:///missing.go")

	assert.Equal(t, []string{"file:///a.go"}, d.List())
}

func TestOpenDocs_ListEmpty(t *testing.T) {
	d := newOpenDocs()
	assert.Empty(t, d.List())
}

func TestOpenDocs_ConcurrentAddList(t *testing.T) {
	d := newOpenDocs()
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			d.Add("file:///" + strconv.Itoa(n) + ".go")
			_ = d.List()
			d.Remove("file:///" + strconv.Itoa(n) + ".go")
		}(i)
	}
	wg.Wait()
}
