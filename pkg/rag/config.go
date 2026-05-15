package rag

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
