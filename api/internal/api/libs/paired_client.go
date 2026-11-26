package libs

import "github.com/gin-gonic/gin"

const pairedClientKey = "libs.pairedClient"

// PairedClient is the device a bearer-authenticated request came from.
type PairedClient struct {
	ID   string
	Name string
}

// SetPairedClient records who the bearer guard authenticated for later handlers.
func SetPairedClient(c *gin.Context, client PairedClient) {
	c.Set(pairedClientKey, client)
}

// PairedClientFrom reports the device behind the request; false on the local socket.
func PairedClientFrom(c *gin.Context) (PairedClient, bool) {
	client, ok := c.Get(pairedClientKey)
	if !ok {
		return PairedClient{}, false
	}
	paired, ok := client.(PairedClient)
	return paired, ok
}
