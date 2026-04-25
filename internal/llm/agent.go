package llm

import (
	"context"
	"encoding/json"
)

type Agent struct {
	Spec     AgentSpec
	Executor Executor
	LLM      Client
}

// Run executes a single LLM-driven action.
// state is passed to the executor to provide context-specific information.
func (a *Agent) Run(ctx context.Context, input string, state ExecutorState) (any, error) {
	prompt := BuildPrompt(a.Spec, input)

	resp, err := a.LLM.Call(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var call struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	}

	if err := json.Unmarshal(resp, &call); err != nil {
		return nil, err
	}

	return a.Executor.Execute(ctx, call.Name, call.Args, state)
}
