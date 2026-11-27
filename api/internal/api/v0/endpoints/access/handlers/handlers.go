package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/exposure"
)

type Access interface {
	Status() exposure.Status
	Enable(address string) (exposure.Status, error)
	Disable() (exposure.Status, error)
}

type Handlers struct {
	access Access
}

func New(access Access) *Handlers {
	return &Handlers{access: access}
}

type accessResponse struct {
	Enabled     bool     `json:"enabled"`
	Address     string   `json:"address"`
	Reachable   []string `json:"reachable"`
	Fingerprint string   `json:"fingerprint"`
	Error       string   `json:"error,omitempty"`
}

type putRequest struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
}

func respond(st exposure.Status) accessResponse {
	reach := st.Reachable
	if reach == nil {
		reach = []string{}
	}
	return accessResponse{
		Enabled: st.Enabled, Address: st.Address, Reachable: reach,
		Fingerprint: st.Fingerprint, Error: st.Error,
	}
}

func (h *Handlers) Get(c *gin.Context) {
	libs.WriteQueryOK(c, respond(h.access.Status()))
}

func (h *Handlers) Put(c *gin.Context) {
	if c.Request.TLS != nil {
		libs.WriteErr(c, http.StatusForbidden, "device connections can only be changed from this computer")
		return
	}
	var req putRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if !req.Enabled {
		st, err := h.access.Disable()
		if err != nil {
			libs.WriteErr(c, http.StatusInternalServerError, err.Error())
			return
		}
		libs.WriteQueryOK(c, respond(st))
		return
	}
	st, err := h.access.Enable(req.Address)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	libs.WriteQueryOK(c, respond(st))
}
