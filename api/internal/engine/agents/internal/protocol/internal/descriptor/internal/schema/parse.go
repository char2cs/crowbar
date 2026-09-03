package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func parse(raw []byte) (Vocabulary, error) {
	var v Vocabulary
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return Vocabulary{}, fmt.Errorf("schema: vocabulary: %w", err)
	}
	return v, nil
}
