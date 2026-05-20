package catalog

import (
	"embed"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultCatalog embed.FS

var tierOrder = map[string]int{
	"core":     0,
	"standard": 1,
	"extended": 2,
	"runtime":  3,
}

func LoadDefault() (*ToolCatalog, error) {
	data, err := defaultCatalog.ReadFile("default.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded catalog: %w", err)
	}
	return parse(data)
}

func LoadFromFile(path string) (*ToolCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog %s: %w", path, err)
	}
	return parse(data)
}

func parse(data []byte) (*ToolCatalog, error) {
	var cat ToolCatalog
	if err := yaml.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	return &cat, nil
}

func (c *ToolCatalog) Lookup(name string) (ToolEntry, bool) {
	entry, ok := c.Tools[name]
	return entry, ok
}

func (c *ToolCatalog) CoreTools() []string {
	var names []string
	for name, entry := range c.Tools {
		if entry.Tier == "core" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (c *ToolCatalog) HighestTier(tools []string) string {
	highest := "core"
	for _, name := range tools {
		entry, ok := c.Tools[name]
		if !ok {
			continue
		}
		if tierOrder[entry.Tier] > tierOrder[highest] {
			highest = entry.Tier
		}
	}
	return highest
}

func (c *ToolCatalog) MaxRiskScore(tools []string) int {
	max := 0
	for _, name := range tools {
		entry, ok := c.Tools[name]
		if !ok {
			continue
		}
		if entry.Risk.Score > max {
			max = entry.Risk.Score
		}
	}
	return max
}

func (c *ToolCatalog) PackageNames(tools []string, distro string) []string {
	var pkgs []string
	for _, name := range tools {
		entry, ok := c.Tools[name]
		if !ok {
			continue
		}
		if pkg, ok := entry.Package[distro]; ok {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

func (c *ToolCatalog) Validate(tools []string) error {
	for _, name := range tools {
		if _, ok := c.Tools[name]; !ok {
			return fmt.Errorf("unknown tool %q not in catalog", name)
		}
	}
	return nil
}
