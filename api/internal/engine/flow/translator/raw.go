package translator

type rawAgent struct {
	Intelligence string   `yaml:"intelligence"`
	SystemPrompt string   `yaml:"system_prompt"`
	Tools        []string `yaml:"tools"`
}

type rawTransition struct {
	On string `yaml:"on"`
	To string `yaml:"to"`
}

type rawEmit struct {
	Agent string `yaml:"agent"`
	On    string `yaml:"on"`
}

type rawState struct {
	Name        string          `yaml:"name"`
	Agent       *rawAgent       `yaml:"agent"`
	UI          string          `yaml:"ui"`
	Items       bool            `yaml:"items"`
	Terminal    bool            `yaml:"terminal"`
	Transitions []rawTransition `yaml:"transitions"`
	Emits       []rawEmit       `yaml:"emits"`
}

type rawFlow struct {
	Name         string     `yaml:"name"`
	Version      string     `yaml:"version"`
	Description  string     `yaml:"description"`
	ItemStatuses []string   `yaml:"item_statuses"`
	States       []rawState `yaml:"states"`
}
