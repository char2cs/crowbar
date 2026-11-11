package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
)

// A handler built without a crowbar home would resolve one of its own later,
// so New refuses it up front.
func TestNew_NilCrowbarHomePanics(
	t *testing.T,
) {
	assert.Panics(t, func() {
		projecthandlers.New(&fakeReader{}, &fakeImporter{}, &fakeDeleter{}, nil, nil)
	})
}
