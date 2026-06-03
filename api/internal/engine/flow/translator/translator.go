package translator

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/flow/def"
)

//go:embed schema/flow.json
var schemaJSON []byte

var compiledSchema *jsonschema.Schema

func init() {
	compiledSchema = mustCompileSchema(schemaJSON)
}

// mustCompileSchema parses and compiles the JSON schema from raw bytes, panicking on any failure.
func mustCompileSchema(data []byte) *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		panic(fmt.Sprintf("flow translator: failed to parse embedded schema: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("flow.json", doc); err != nil {
		panic(fmt.Sprintf("flow translator: failed to add schema resource: %v", err))
	}
	s, err := c.Compile("flow.json")
	if err != nil {
		panic(fmt.Sprintf("flow translator: failed to compile schema: %v", err))
	}
	return s
}

// Parse reads raw YAML bytes, validates the structure against the embedded JSON
// schema, and maps the result to a def.FlowDefinition.
func Parse(data []byte) (def.FlowDefinition, error) {
	// Step 1: unmarshal YAML into a generic map for schema validation.
	var rawDoc any
	if err := yaml.Unmarshal(data, &rawDoc); err != nil {
		return def.FlowDefinition{}, fmt.Errorf("flow translator: yaml unmarshal: %w", err)
	}

	// Convert yaml.v3 output to JSON-compatible form.
	jsonCompatible, err := toJSONCompatible(rawDoc)
	if err != nil {
		return def.FlowDefinition{}, fmt.Errorf("flow translator: yaml→json conversion: %w", err)
	}

	// Step 2: validate against JSON schema.
	if err := compiledSchema.Validate(jsonCompatible); err != nil {
		return def.FlowDefinition{}, fmt.Errorf("flow translator: schema validation: %w", err)
	}

	// Step 3: unmarshal YAML into typed raw structs.
	// Cannot fail: data already unmarshalled successfully in step 1.
	var raw rawFlow
	_ = yaml.Unmarshal(data, &raw)

	return mapFlow(raw), nil
}

// toJSONCompatible round-trips through JSON encoding to produce a form that the
// JSON schema validator can process.
func toJSONCompatible(v any) (any, error) {
	jsonBytes, err := json.Marshal(convertYAMLNode(v))
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(strings.NewReader(string(jsonBytes)))
}

// convertYAMLNode recursively normalises yaml.v3-decoded values to ensure all
// map keys are strings (JSON-compatible).
func convertYAMLNode(v any) any {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, cv := range val {
			out[k] = convertYAMLNode(cv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, cv := range val {
			out[i] = convertYAMLNode(cv)
		}
		return out
	default:
		return val
	}
}
