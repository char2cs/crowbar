package spec

type AnswerSpec map[string]AnswerEventSpec

type AnswerEventSpec struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`

	AnswersInto string `yaml:"answers_into"`

	Responses map[string]string `yaml:"responses"`
}

func (a AnswerSpec) Event(canonical string) (AnswerEventSpec, bool) {
	s, ok := a[canonical]
	if !ok || len(s.Responses) == 0 {

		return AnswerEventSpec{}, false
	}
	return s, true
}
