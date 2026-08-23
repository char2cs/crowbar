package spec

const (
	InjectedPromptTaskNotification = "task_notification"
)

var InjectedPromptKinds = map[string]struct{}{
	InjectedPromptTaskNotification: {},
}

type InjectedPromptSpec struct {
	Kind string `yaml:"kind"`

	Needle string `yaml:"needle"`
}
