package models

type Selection struct {
	Model  string
	Effort string
}

func (s Selection) Empty() bool {
	return s.Model == "" && s.Effort == ""
}
