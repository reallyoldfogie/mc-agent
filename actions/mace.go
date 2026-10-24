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

type MaceSmashWithApproach struct{}

func (MaceSmashWithApproach) Name() string  { return "macesmashwithapproach" }
func (MaceSmashWithApproach) Usage() string { return "maceSmashWithApproach <entityID> <itemName>" }

// Execute mirrors MaceSmash's own argument parsing exactly; the only
// difference is which method it dispatches to (MaceSmashAtWithApproach
// instead of MaceSmashAt) — see that method's own doc comment for why a
// separate command exists rather than reusing "macesmash" (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 7).
func (MaceSmashWithApproach) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	if len(args) != 2 {
		_ = agent.SendChat("Usage: maceSmashWithApproach <entityID> <itemName>")
		return models.Done(nil), nil
	}
	targetID, err := parseCombatEntityID(args[0], agent)
	if err != nil {
		return models.Done(nil), nil
	}
	smasher, ok := agent.(interface {
		MaceSmashAtWithApproach(context.Context, int32, string) error
	})
	if !ok {
		return models.Done(fmt.Errorf("mace smash with approach action is not supported by this agent")), nil
	}
	completion, resolve := models.NewCompletion()
	go func() { resolve(smasher.MaceSmashAtWithApproach(ctx, targetID, args[1])) }()
	return completion, nil
}
