package manifest

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func ParseFile(path string) (*AgentManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (*AgentManifest, error) {
	var m AgentManifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := Validate(m); err != nil {
		return nil, err
	}
	return &m, nil
}

func Validate(m AgentManifest) error {
	if m.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if m.Spec.Base.Image == "" {
		return fmt.Errorf("spec.base.image is required")
	}
	if m.Spec.Prompt.Text == "" && m.Spec.Prompt.Source == nil {
		return fmt.Errorf("spec.prompt.text or spec.prompt.source is required")
	}
	if m.Spec.Prompt.Text != "" && m.Spec.Prompt.Source != nil {
		return fmt.Errorf("spec.prompt.text and spec.prompt.source are mutually exclusive")
	}
	return nil
}
