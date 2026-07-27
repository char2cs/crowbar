package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/perf"
)

// PerfData is the response payload for the /system/perf routes: the ring's
// arming state plus its contents, oldest sample first.
type PerfData struct {
	Enabled bool          `json:"enabled"`
	Samples []perf.Sample `json:"samples"`
}

// PerfHandler exposes the daemon's timing ring so a capture run can read it
// from outside the process. It carries no state of its own — the ring is
// package-level in perf — but is a type to match the endpoint's other handler
// and to keep the routes table uniform.
type PerfHandler struct{}

// NewPerfHandler returns a handler for the /system/perf routes.
func NewPerfHandler() *PerfHandler {
	return &PerfHandler{}
}

// Get is the GET /system/perf gin handler. It answers the ring's contents even
// while disarmed, so a reader can tell "armed and quiet" from "never armed".
func (h *PerfHandler) Get(
	c *gin.Context,
) {
	libs.WriteQueryOK(c, PerfData{
		Enabled: perf.Enabled(),
		Samples: samplesOrEmpty(),
	})
}

// Set is the POST /system/perf?enabled=true|false gin handler. Arming always
// clears the ring so a capture run starts from a known-empty window and no
// sample from a previous scenario is attributed to this one.
//
// An absent or unparseable enabled value is a 400 rather than a silent
// disarm: treating a typo as "false" would let an operator drive a whole
// scenario and read back an empty ring with nothing to explain why.
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

// samplesOrEmpty keeps an empty ring serialising as [] rather than null, so a
// reader can index the result without a nil check.
func samplesOrEmpty() []perf.Sample {
	samples := perf.Snapshot()
	if samples == nil {
		return []perf.Sample{}
	}
	return samples
}
