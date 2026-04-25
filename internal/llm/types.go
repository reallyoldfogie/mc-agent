package llm

import "context"

type AgentSpec struct {
	Interface string     `json:"interface"`
	Tools     []ToolSpec `json:"tools"`
}

type ToolSpec struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  map[string]string `json:"parameters"`
	Returns     map[string]string `json:"returns"`
}

type ExecutorState interface {
	// Marker interface for executor state implementations
	// Specific executor implementations cast to their own state type
}

type Executor interface {
	Execute(ctx context.Context, name string, args map[string]any, state ExecutorState) (any, error)
}

type CommandMetadata struct {
	Context   []int  `json:"context,omitempty"`
	Response  string `json:"response,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}
