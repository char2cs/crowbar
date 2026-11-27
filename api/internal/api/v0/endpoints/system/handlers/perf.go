package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/perf"
)

type PerfData struct {
	Enabled bool          `json:"enabled"`
	Samples []perf.Sample `json:"samples"`
}

type PerfHandler struct{}

func NewPerfHandler() *PerfHandler {
	return &PerfHandler{}
}

func (h *PerfHandler) Get(
	c *gin.Context,
) {
	libs.WriteQueryOK(c, PerfData{
		Enabled: perf.Enabled(),
		Samples: samplesOrEmpty(),
	})
}

func (h *PerfHandler) Set(
	c *gin.Context,
) {
	on, err := strconv.ParseBool(c.Query("enabled"))
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "enabled query parameter must be true or false")
		return
	}

	perf.Reset()
	perf.SetEnabled(on)
	libs.WriteQueryOK(c, PerfData{
		Enabled: on,
		Samples: []perf.Sample{},
	})
}

func samplesOrEmpty() []perf.Sample {
	samples := perf.Snapshot()
	if samples == nil {
		return []perf.Sample{}
	}
	return samples
}
