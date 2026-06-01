package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// SkillMeta holds the metadata from a SKILL.md frontmatter.
type SkillMeta struct {
	Name        string
	Description string
	Dir         string
}

// Discover recursively scans the skills directory for subdirectories
// containing SKILL.md files and returns their metadata.
func Discover(skillsDir string) ([]SkillMeta, error) {
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var skills []SkillMeta

	err := filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden directories (e.g. ..data, ..2024_01_01 created
		// by Kubernetes ConfigMap volume mounts)
		if d.IsDir() && strings.HasPrefix(d.Name(), "..") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		dir := filepath.Dir(path)

		meta, err := ParseFrontmatter(path)
		if err != nil {
			slog.Warn("skipping skill with invalid SKILL.md", "path", path, "error", err)
			return nil
		}
		meta.Dir = dir

		cardPath := filepath.Join(dir, "skill.yaml")
		if _, statErr := os.Stat(cardPath); statErr == nil {
			if sy, parseErr := ParseSkillYAML(cardPath); parseErr == nil {
				if sy.Metadata.Description != "" {
					meta.Description = sy.Metadata.Description
				}
			} else {
				slog.Warn("skill.yaml exists but failed to parse", "dir", dir, "error", parseErr)
			}
		}

		skills = append(skills, meta)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk skills directory: %w", err)
	}

	return skills, nil
}

// LoadContent reads the full content of a skill's SKILL.md file.
// It searches recursively under skillsDir for a directory matching
// the skill name that contains a SKILL.md file.
func LoadContent(skillsDir, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid skill name: %q", name)
	}

	// Try direct path first (backward compatible)
	path := filepath.Join(skillsDir, name, "SKILL.md")
	if data, err := os.ReadFile(path); err == nil {
		return fmt.Sprintf("=== SKILL: %s ===\n%s", name, string(data)), nil
	}

	// Search recursively
	var found string
	_ = filepath.WalkDir(skillsDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "SKILL.md" && filepath.Base(filepath.Dir(p)) == name {
			found = p
			return filepath.SkipAll
		}
		return nil
	})

	if found == "" {
		return "", fmt.Errorf("skill '%s' not found", name)
	}

	data, err := os.ReadFile(found)
	if err != nil {
		return "", fmt.Errorf("skill '%s': %w", name, err)
	}
	return fmt.Sprintf("=== SKILL: %s ===\n%s", name, string(data)), nil
}

// BuildSummary creates a text block listing all available skills.
func BuildSummary(skills []SkillMeta) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\nAvailable skills (call load_skill to get full instructions):\n")
	for _, s := range skills {
		fmt.Fprintf(&sb, "  - %s: %s\n", s.Name, s.Description)
	}
	return sb.String()
}

// ParseFrontmatter extracts name and description from YAML
// frontmatter in a SKILL.md file.
func ParseFrontmatter(path string) (SkillMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillMeta{}, err
	}

	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return SkillMeta{}, fmt.Errorf("no frontmatter found")
	}

	end := strings.Index(content[3:], "---")
	if end == -1 {
		return SkillMeta{}, fmt.Errorf("unclosed frontmatter")
	}

	frontmatter := content[3 : end+3]
	meta := SkillMeta{}

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			meta.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
		if strings.HasPrefix(line, "description:") {
			meta.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}

	if meta.Name == "" {
		return SkillMeta{}, fmt.Errorf("skill has no name")
	}

	return meta, nil
}
