package llm

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

	ollama_api "github.com/ollama/ollama/api"
)

type ollamaClient struct {
	client      *ollama_api.Client
	model       string
	lastContext []int // Context from last response to reuse for conversation continuity
}

func NewOllamaClient(model string) Client {
	var err error
	llmClient := &ollamaClient{
		model: model,
	}
	llmClient.client, err = ollama_api.ClientFromEnvironment()
	if err != nil {
		panic(err)
	}

	return llmClient
}

// Call implements the Client interface.
// Takes a prompt as []byte, sends it to Ollama, and returns the raw response JSON as []byte.
func (c *ollamaClient) Call(ctx context.Context, prompt []byte) ([]byte, error) {
	log.Printf("Sending prompt to LLM (context tokens: %d)", len(c.lastContext))

	req := &ollama_api.GenerateRequest{
		Model:   c.model,
		Prompt:  string(prompt),
		Stream:  new(bool),     // false
		Context: c.lastContext, // Reuse context from previous response
	}

	var respBuffer bytes.Buffer

	respFunc := func(resp ollama_api.GenerateResponse) error {
		respBuffer.WriteString(resp.Response)
		// Save context for next request
		c.lastContext = resp.Context
		return nil
	}

	err := c.client.Generate(ctx, req, respFunc)
	if err != nil {
		return nil, err
	}

	jsonResp := extractJSON(respBuffer.Bytes())
	if jsonResp == nil {
		return nil, fmt.Errorf("no valid JSON found in response: %s", respBuffer.String())
	}

	fmt.Printf("\n============\n LLM raw request: %s \n============\n LLM raw response: %s \n=============\n", string(prompt), respBuffer.String())
	return jsonResp, nil
}

func extractJSON(input []byte) []byte {
	start := strings.Index(string(input), "{")
	end := strings.LastIndex(string(input), "}")

	if start == -1 || end == -1 || end < start {
		return nil // No valid JSON object found
	}
	return input[start : end+1]
}
