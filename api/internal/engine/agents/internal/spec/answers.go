package spec

type AnswerEventSpec struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`

	AnswersInto string `yaml:"answers_into"`

	Responses map[string]string `yaml:"responses"`
}
