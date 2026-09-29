package actions

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

type creeperActionAgent struct {
	models.CommandAgent
	err error
}

func (a *creeperActionAgent) KillCreeperForGunpowder(context.Context) (bool, error) {
	return false, a.err
}

func TestKillCreeperForGunpowderActionAcceptsNoDrop(t *testing.T) {
	a := &creeperActionAgent{}
	completion, err := (KillCreeperForGunpowder{}).Execute(context.Background(), a, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if err := completion.Wait(context.Background()); err != nil {
		t.Fatalf("completion.Wait() error = %v, want success for a valid no-drop result", err)
	}
}

func TestKillCreeperForGunpowderActionPropagatesEncounterError(t *testing.T) {
	wantErr := context.Canceled
	a := &creeperActionAgent{err: wantErr}
	completion, err := (KillCreeperForGunpowder{}).Execute(context.Background(), a, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if err := completion.Wait(context.Background()); err != wantErr {
		t.Fatalf("completion.Wait() error = %v, want %v", err, wantErr)
	}
}
