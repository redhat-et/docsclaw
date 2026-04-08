package main

import (
	"github.com/redhat-et/docsclaw/internal/cmd"

	// Register LLM provider implementations
	_ "github.com/redhat-et/docsclaw/internal/anthropic"
	_ "github.com/redhat-et/docsclaw/internal/openai"
)

func main() {
	cmd.Execute()
}
