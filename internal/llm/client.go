package llm

import "context"

type Client interface {
	Call(ctx context.Context, prompt []byte) ([]byte, error)
}
