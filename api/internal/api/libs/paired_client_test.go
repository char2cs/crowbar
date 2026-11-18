package libs_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func TestPairedClient_RoundTripsAndIsAbsentByDefault(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, ok := libs.PairedClientFrom(c)
	assert.False(t, ok)

	libs.SetPairedClient(c, libs.PairedClient{ID: "c1", Name: "phone"})
	got, ok := libs.PairedClientFrom(c)
	assert.True(t, ok)
	assert.Equal(t, libs.PairedClient{ID: "c1", Name: "phone"}, got)
}
