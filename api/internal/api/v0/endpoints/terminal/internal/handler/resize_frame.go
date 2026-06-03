package handler

// resizeFrame is the JSON shape the client sends to resize the PTY window.
type resizeFrame struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}
