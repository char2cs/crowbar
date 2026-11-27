package libs

import "github.com/gin-gonic/gin"

const pairedClientKey = "libs.pairedClient"

type PairedClient struct {
	ID   string
	Name string
}

func SetPairedClient(c *gin.Context, client PairedClient) {
	c.Set(pairedClientKey, client)
}

func PairedClientFrom(c *gin.Context) (PairedClient, bool) {
	client, ok := c.Get(pairedClientKey)
	if !ok {
		return PairedClient{}, false
	}
	paired, ok := client.(PairedClient)
	return paired, ok
}
