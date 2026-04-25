package llm

import (
	"encoding/json"
	"fmt"
)

func BuildPrompt(spec AgentSpec, input string) []byte {
	payload := map[string]any{
		"task":  input,
		"tools": spec.Tools,
		"rules": []string{
			"Return JSON only",
			"Do not invent functions",
			"Match parameter types",
		},
	}

	data, _ := json.MarshalIndent(payload, "", "  ")
	return data
}

// BuildAgentPrompt constructs a rich prompt for the Minecraft agent, including game state.
// goal: the high-level objective (e.g., "Find and mine logs")
// gameState: formatted game context (position, inventory, visible blocks, etc.)
// Returns the full prompt as a formatted string that will be sent to the LLM.
func BuildAgentPrompt(spec AgentSpec, goal, gameState string) string {
	toolsJSON, _ := json.MarshalIndent(spec.Tools, "", "  ")

	return fmt.Sprintf(`You control a Minecraft agent. Your goal is: %s

You have access to the following tools (functions):
%s

Game State:
%s

Respond with a JSON action to take next. The JSON must have this format:
{
  "name": "<tool_name>",
  "args": {
    "<param_name>": <value>,
    ...
  }
}

Rules:
- Return JSON only
- Do not invent functions
- Match parameter types exactly
- Use the tool names and parameter names exactly as specified above
- Each action is a single tool call

`, goal, string(toolsJSON), gameState)
}
