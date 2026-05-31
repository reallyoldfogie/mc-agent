package agent

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/reallyoldfogie/mc-agent/internal/llm"
	"github.com/reallyoldfogie/mc-agent/models"
)

// AgentExecutorState holds orchestration state for the AgentExecutor.
type AgentExecutorState struct {
	LastPath *models.Path
}

type AgentExecutor struct {
	agent models.Agent
}

// NewLLMExecutor creates a new executor for LLM tool calls.
// The executor is stateless; orchestration state is managed by the caller.
func NewLLMExecutor(a models.Agent) *AgentExecutor {
	return &AgentExecutor{
		agent: a,
	}
}

// Execute implements the llm.Executor interface.
// It dispatches LLM action calls to the corresponding agent methods.
// state can be cast to *AgentExecutorState to access orchestration state.
func (e *AgentExecutor) Execute(ctx context.Context, name string, args map[string]any, state llm.ExecutorState) (any, error) {
	log.Printf("AgentExecutor.Execute: action=%s, args=%v", name, args)

	switch name {
	case "FindPath":
		return e.executeFindPath(ctx, args)
	case "ExecutePath":
		return e.executeExecutePath(ctx, args, state)
	case "MineBlockAt":
		return e.executeMineBlockAt(ctx, args)
	case "FindAllVisibleBlocksInSphere":
		return e.executeFindAllVisibleBlocksInSphere(ctx, args)
	case "exit":
		return e.executeExit(ctx)
	default:
		return nil, fmt.Errorf("unknown action: %s", name)
	}
}

func (e *AgentExecutor) executeFindPath(ctx context.Context, args map[string]any) (any, error) {
	x, okX := args["x"].(float64)
	y, okY := args["y"].(float64)
	z, okZ := args["z"].(float64)

	if !okX || !okY || !okZ {
		return nil, fmt.Errorf("FindPath requires x, y, z as float64")
	}

	path, err := e.agent.FindPath(ctx, x, y, z)
	if err != nil {
		return nil, fmt.Errorf("pathfind (%.2f, %.2f, %.2f) failed: %v", x, y, z, err)
	}

	return path, nil
}

func (e *AgentExecutor) executeExecutePath(ctx context.Context, _ map[string]any, state llm.ExecutorState) (any, error) {
	execState, ok := state.(*AgentExecutorState)
	if !ok || execState == nil || execState.LastPath == nil {
		return "No precomputed path to execute", nil
	}

	err := e.agent.ExecutePath(ctx, execState.LastPath)
	if err != nil {
		return fmt.Sprintf("Execute path failed: %v", err), err
	}

	return "Successfully executed path", nil
}

func (e *AgentExecutor) executeMineBlockAt(ctx context.Context, args map[string]any) (any, error) {
	// Extract pos as map ({"x": ..., "y": ..., "z": ...})
	posMap, ok := args["pos"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("MineBlockAt requires 'pos' as a map with x, y, z")
	}

	x, okX := posMap["x"].(float64)
	y, okY := posMap["y"].(float64)
	z, okZ := posMap["z"].(float64)

	if !okX || !okY || !okZ {
		return nil, fmt.Errorf("MineBlockAt pos requires x, y, z as float64")
	}

	target := models.V3{X: x, Y: y, Z: z}

	// Check mining range
	agentPos, initialized := e.agent.GetPositionSimple()
	if !initialized {
		return nil, fmt.Errorf("agent position not initialized")
	}

	if agentPos.DistanceTo(target) > 3.0 {
		return fmt.Sprintf("Target block at (%.2f, %.2f, %.2f) is out of mining range", x, y, z), nil
	}

	blockAt := e.agent.BlockNameAt(int(x), int(y), int(z))
	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	for blockAt != "minecraft:air" {
		err := e.agent.MineBlockAt(timeoutCtx, target, models.FaceUp)
		if err != nil {
			return fmt.Sprintf("Mine (%.2f, %.2f, %.2f) failed: %v", x, y, z, err), err
		}
		time.Sleep(10 * time.Millisecond)

		select {
		case <-timeoutCtx.Done():
			return fmt.Sprintf("Context cancelled while mining block at (%.2f, %.2f, %.2f)[%s]: %v", x, y, z, blockAt, ctx.Err()), timeoutCtx.Err()
		default:
			// continue mining
		}

		blockAt = e.agent.BlockNameAt(int(x), int(y), int(z))
	}

	// Wait for item entity to spawn
	time.Sleep(500 * time.Millisecond)

	// Get item entity type ID
	itemEntityTypeID, _ := e.agent.GetEntityTypeID("minecraft:item")

	// Check for item entities
	const itemPickupRange = 1.5
	for attempt := 0; attempt < 20; attempt++ {
		entities := e.agent.GetTrackedEntities()

		itemFound := false
		for _, entityInfo := range entities {
			if entityInfo.EntityType == itemEntityTypeID {
				dist := target.DistanceTo(models.V3{X: entityInfo.X, Y: entityInfo.Y, Z: entityInfo.Z})
				if dist < 10 {
					itemFound = true
					log.Printf("Found item entity at (%.2f, %.2f, %.2f), distance: %.2f", entityInfo.X, entityInfo.Y, entityInfo.Z, dist)

					if dist > itemPickupRange {
						agentPos, _ := e.agent.GetPositionSimple()
						dx := entityInfo.X - agentPos.X
						dz := entityInfo.Z - agentPos.Z
						// Normalize direction vector and move 0.5 blocks closer
						length := math.Sqrt(dx*dx + dz*dz)
						if length > 0 {
							dx /= length
							dz /= length
							targetPos := models.V3{X: agentPos.X + dx*0.5, Y: agentPos.Y, Z: agentPos.Z + dz*0.5}
							path, err := e.agent.FindPath(ctx, targetPos.X, targetPos.Y, targetPos.Z)
							if err == nil {
								e.agent.ExecutePath(ctx, path)
							}
						}
					}
				}
			}
		}

		if !itemFound {
			log.Printf("No item entities found, items likely picked up")
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Sprintf("Successfully mined block at (%.2f, %.2f, %.2f)", x, y, z), nil
}

func (e *AgentExecutor) executeFindAllVisibleBlocksInSphere(ctx context.Context, args map[string]any) (any, error) {
	radiusVal, ok := args["radius"].(float64)
	if !ok {
		return nil, fmt.Errorf("FindAllVisibleBlocksInSphere requires 'radius' as float64")
	}

	radius := int(radiusVal)
	blocks, err := e.agent.FindAllVisibleBlocksInSphere(ctx, radius)
	if err != nil {
		return nil, err
	}

	return blocks, nil
}

func (e *AgentExecutor) executeExit(ctx context.Context) (any, error) {
	log.Printf("Exit command received, shutting down")
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.agent.Close(cleanupCtx)
	os.Exit(0)
	return nil, nil
}
