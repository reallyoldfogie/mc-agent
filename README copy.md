# Geniface Agent Toolchain

A Go-based toolchain for turning interfaces into LLM-callable tools.

This project extracts Go interfaces (including embedded interfaces), generates a flattened schema, and enables an LLM to select and invoke methods dynamically using a runtime agent.

---

## ✨ Features

* Extracts interface methods (including embedded interfaces)
* Supports cross-package interfaces (e.g. `models.Action`)
* Captures method signatures (params + returns)
* Extracts method comments for better LLM reasoning
* Generates structured JSON tool definitions
* Builds LLM-ready prompts automatically
* Executes selected methods via reflection
* Includes argument coercion + validation layer

---

## 📦 Project Structure

```
cmd/geniface/        # CLI for generating interface schema
internal/llm/        # Agent loop + prompt builder
internal/executor/   # Reflection-based method executor
internal/validation/ # Argument coercion + validation
go.mod
```

---

## 🚀 Getting Started

### 1. Generate Interface Schema

Run the CLI to extract an interface:

```
go run ./cmd/geniface -type=models.Action -out=actions.json
```

Supports:

* Local interfaces: `MyInterface`
* Cross-package: `models.Action`
* Full import path: `github.com/you/project/models.Action`

---

### 2. Example Output

```json
{
  "interface": "Action",
  "tools": [
    {
      "name": "Move",
      "description": "Moves the agent to a location",
      "parameters": {
        "x": "int",
        "y": "int"
      },
      "returns": {
        "err": "error"
      }
    }
  ]
}
```

---

### 3. Build Prompt for LLM

```go
prompt := llm.BuildPrompt(spec, "Move the agent to position 10, 20")
```

This produces structured JSON:

```json
{
  "task": "...",
  "tools": [...],
  "rules": [
    "Return JSON only",
    "Do not invent functions",
    "Match parameter types"
  ]
}
```

---

### 4. Run the Agent

```go
agent := llm.Agent{
    Spec:     spec,
    Executor: executor,
    LLM:      client,
}

result, err := agent.Run(ctx, "Move to 10, 20")
```

---

## 🧠 LLM Response Format

The LLM must return:

```json
{
  "name": "Move",
  "args": {
    "x": 10,
    "y": 20
  }
}
```

---

## ⚙️ Executor

The executor maps LLM-selected functions to real Go methods:

```go
executor := &executor.ReflectExecutor{
    Target: myImplementation,
    Spec: map[string]map[string]string{
        "Move": {
            "x": "int",
            "y": "int",
        },
    },
}
```

---

## 🔄 Argument Coercion

Handles common LLM mismatches:

| Input    | Expected | Result |
| -------- | -------- | ------ |
| `"42"`   | int      | 42     |
| `42.0`   | int      | 42     |
| `"true"` | bool     | true   |
| `123`    | string   | "123"  |

---

## ⚠️ Known Limitations

### 1. Parameter Order

Arguments are currently mapped by name but invoked positionally.

If method order matters, consider upgrading to ordered parameter handling.

---

### 2. Limited Type Support

Currently supports:

* `int`
* `string`
* `bool`

Does NOT yet support:

* slices (`[]string`)
* structs
* maps

---

### 3. Embedded Interface Comments

Only top-level interface comments are extracted.

---

## 🧩 Recommended Next Steps

* Add support for:

  * slices and complex types
  * struct decoding via `json.Unmarshal`
* Replace reflection with generated typed bindings
* Add OpenAI/Ollama function-calling schema support
* Implement multi-step planning agent

---

## 💡 Design Philosophy

* Interfaces define capabilities
* JSON defines the contract
* LLM selects the action
* Go executes it safely

---

## 🛠 Example Use Cases

* Autonomous agents (e.g., Minecraft bots)
* Dev tools / CLIs with natural language control
* Workflow automation systems
* Internal AI tooling platforms

---

## 📄 License

MIT (or your preferred license)

---
