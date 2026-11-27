package git

type GitStatus struct {
	WsID   string    `json:"-"`
	Branch string    `json:"branch"`
	Ahead  int       `json:"ahead"`
	Behind int       `json:"behind"`
	Files  []GitFile `json:"files"`
}
