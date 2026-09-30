package actions

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-agent/models"
)

type MaceAttack struct{}

func (MaceAttack) Name() string  { return "maceattack" }
func (MaceAttack) Usage() string { return "maceAttack <entityID> <itemName>" }
func (MaceAttack) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 2 {
		_ = agent.SendChat("Usage: maceAttack <entityID> <itemName>")
		return models.Done(nil), nil
	}
	targetID, err := parseCombatEntityID(args[0], agent)
	if err != nil {
		return models.Done(nil), nil
	}
	attacker, ok := agent.(interface {
		MaceAttackAt(context.Context, int32, string) error
	})
	if !ok {
		return models.Done(fmt.Errorf("mace attack action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(attacker.MaceAttackAt(ctx, targetID, args[1])) }()
	return completion, nil
}

type MaceSmash struct{}

func (MaceSmash) Name() string  { return "macesmash" }
func (MaceSmash) Usage() string { return "maceSmash <entityID> <itemName>" }
func (MaceSmash) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 2 {
		_ = agent.SendChat("Usage: maceSmash <entityID> <itemName>")
		return models.Done(nil), nil
	}
	targetID, err := parseCombatEntityID(args[0], agent)
	if err != nil {
		return models.Done(nil), nil
	}
	smasher, ok := agent.(interface {
		MaceSmashAt(context.Context, int32, string) error
	})
	if !ok {
		return models.Done(fmt.Errorf("mace smash action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(smasher.MaceSmashAt(ctx, targetID, args[1])) }()
	return completion, nil
}
