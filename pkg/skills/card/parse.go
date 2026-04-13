package card

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// nameRegex enforces Agent Skills naming rules:
// - Lowercase a-z, digits 0-9, hyphens only
// - 1-64 characters
// - Must not start or end with hyphen
// - Must not contain consecutive hyphens
var nameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// Parse reads a skill.yaml file, unmarshals it, and validates the result.
func Parse(path string) (SkillCard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillCard{}, fmt.Errorf("failed to read skill.yaml: %w", err)
	}

	var sc SkillCard
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return SkillCard{}, fmt.Errorf("failed to unmarshal skill.yaml: %w", err)
	}

	if err := Validate(sc); err != nil {
		return SkillCard{}, err
	}

	return sc, nil
}

// Validate checks required fields and enforces Agent Skills naming rules.
func Validate(sc SkillCard) error {
	if sc.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}

	if sc.Kind != "SkillCard" {
		return fmt.Errorf("kind must be \"SkillCard\", got %q", sc.Kind)
	}

	if sc.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if !nameRegex.MatchString(sc.Metadata.Name) {
		return fmt.Errorf("metadata.name %q does not match Agent Skills naming rules (lowercase a-z, digits 0-9, hyphens, 1-64 chars, no leading/trailing/consecutive hyphens)", sc.Metadata.Name)
	}

	// Additional check for consecutive hyphens
	if regexp.MustCompile(`--`).MatchString(sc.Metadata.Name) {
		return fmt.Errorf("metadata.name %q contains consecutive hyphens", sc.Metadata.Name)
	}

	if sc.Metadata.Namespace == "" {
		return fmt.Errorf("metadata.namespace is required")
	}

	if sc.Metadata.Ref == "" {
		return fmt.Errorf("metadata.ref is required")
	}

	if sc.Metadata.Version == "" {
		return fmt.Errorf("metadata.version is required")
	}

	if sc.Metadata.Description == "" {
		return fmt.Errorf("metadata.description is required")
	}

	if len(sc.Metadata.Description) > 1024 {
		return fmt.Errorf("metadata.description exceeds 1024 characters (%d)", len(sc.Metadata.Description))
	}

	if sc.Metadata.Author == "" {
		return fmt.Errorf("metadata.author is required")
	}

	return nil
}
