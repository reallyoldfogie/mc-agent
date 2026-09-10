package courier

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-agent/models"
)

// ChatResponder is the narrow slice of models.CommandAgent TransferAction
// actually needs - just enough to reply to whoever triggered it. Kept
// narrow, like AgentMessenger, so the real dispatch logic (run, below) is
// unit-testable without a full models.CommandAgent fake - that interface is
// large (~35 methods across MovementAgent/ChatOperations/CommandAgent's own
// list) because it's shared by every other chat-driven action, not because
// Transfer needs any of that.
type ChatResponder interface {
	SendChat(message string) error
}

// TransferAction is a chat/RL/LLM-dispatchable command - "transfer <slot>
// <destLabel>" - bound to one particular server label as its implicit
// source. Register one instance per configured server, via
// models.ActionRegistrar.RegisterAction on that server's own agent, so a
// command addressed to that server's bot (the existing
// ">>>botName<<<transfer ..." chat convention - see agent/commands.go's
// handleChatCommand) - or dispatched the same way by an RL/LLM executor
// driving the identical models.ActionRegistry[models.CommandAgent]
// mechanism - starts a transfer sourced from that server. This one
// implementation satisfies all three trigger surfaces named in
// docs/plans/ITEM_TRANSFER_COURIER_PLAN.md's Open Question 1 (chat, direct
// API via Courier.StartTransfer, RL/LLM), since chat and RL/LLM dispatch
// already share this exact registry mechanism in this codebase - see
// models.ActionRegistrar's doc comment.
type TransferAction struct {
	Courier     *Courier
	SourceLabel string
}

func (t TransferAction) Name() string { return "transfer" }

func (t TransferAction) Usage() string {
	return fmt.Sprintf("transfer <slot> <destLabel> - move the item in <slot> from %s to <destLabel>", t.SourceLabel)
}

// Execute implements models.Action[models.CommandAgent]. It delegates to
// run against just the ChatResponder slice of agent Transfer actually
// needs - agent (a models.CommandAgent) always satisfies ChatResponder,
// since CommandAgent embeds ChatOperations.
func (t TransferAction) Execute(ctx context.Context, agent models.CommandAgent, args []string) (models.Completion, error) {
	return t.run(ctx, agent, args)
}

// run does the actual work; it's the unit-tested unit (see action_test.go),
// independent of the large models.CommandAgent interface Execute is forced
// to accept.
func (t TransferAction) run(ctx context.Context, chat ChatResponder, args []string) (models.Completion, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("usage: %s", t.Usage())
	}
	slotID, destLabel := args[0], args[1]

	completion, resolve := models.NewCompletion()
	_ = chat.SendChat(fmt.Sprintf("Starting transfer: slot %s -> %s...", slotID, destLabel))
	go func() {
		outcome, err := t.Courier.StartTransfer(ctx, t.SourceLabel, destLabel, slotID)
		if err != nil {
			_ = chat.SendChat("Transfer failed: " + err.Error())
			resolve(err)
			return
		}
		_ = chat.SendChat(fmt.Sprintf("Transfer complete (jti=%s)", outcome.JTI))
		resolve(nil)
	}()
	return completion, nil
}
