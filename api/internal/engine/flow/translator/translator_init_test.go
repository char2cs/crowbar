package translator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMustCompileSchema_InvalidJSON_Panics(t *testing.T) {
	assert.Panics(t, func() {
		mustCompileSchema([]byte(`not valid json {{{`))
	})
}
