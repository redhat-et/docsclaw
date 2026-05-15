package rag

import "fmt"

type Config struct {
	Backend      string `yaml:"backend"`
	URL          string `yaml:"url"`
	Collection   string `yaml:"collection"`
	TextField    string `yaml:"text_field"`
	DefaultLimit int    `yaml:"default_limit"`
	MaxLimit     int    `yaml:"max_limit"`
}

func (c *Config) ApplyDefaults() {
	if c.TextField == "" {
		c.TextField = "content"
	}
	if c.DefaultLimit <= 0 {
		c.DefaultLimit = 5
	}
	if c.MaxLimit <= 0 {
		c.MaxLimit = 20
	}
}

func NewClient(cfg *Config) (Client, error) {
	cfg.ApplyDefaults()
	switch cfg.Backend {
	case "weaviate":
		return NewWeaviateClient(cfg.URL, cfg.Collection, cfg.TextField)
	default:
		return nil, fmt.Errorf("unsupported RAG backend: %q", cfg.Backend)
	}
}
