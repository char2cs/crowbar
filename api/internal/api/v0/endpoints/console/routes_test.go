package console_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	consoleendpoint "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/console"
	"github.com/char2cs/crowbar/api/internal/core/logring"
)

func TestRegister_Routes_MountsTheLogStream(
	t *testing.T,
) {
	r := gin.New()

	consoleendpoint.Register(r.Group("/v0"), logring.New())

	var routes []string
	for _, route := range r.Routes() {
		routes = append(routes, route.Method+" "+route.Path)
	}
	assert.Equal(t, []string{"GET /v0/console/logs"}, routes)
}
