package models

// Selection is one chat's declared choice of model and reasoning effort.
//
// An EMPTY field means "the provider's own default", which is NOT the same fact
// as any declared value and is never substituted for one: nothing is appended to
// the argv for it, so an unselected chat launches exactly the command line it
// launched before this type existed. That is also why the zero Selection is a
// legitimate value rather than a missing one — it is what every chat holds until
// a user chooses otherwise, and what clearing a choice returns it to.
type Selection struct {
	Model  string
	Effort string
}

// Empty reports a selection that asks for nothing.
func (s Selection) Empty() bool {
	return s.Model == "" && s.Effort == ""
}
