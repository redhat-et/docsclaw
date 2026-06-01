package source

import "context"

type Skill struct {
	Name    string
	Content []byte
}

type PullOptions struct {
	Version string
}

type Source interface {
	Pull(ctx context.Context, ref string, opts PullOptions) (*Skill, error)
}
