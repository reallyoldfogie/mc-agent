package v1_21_11

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.11/play/clientbound"
	protocolmodels "github.com/reallyoldfogie/mc-protocol-go/models"
)

func TestPlayHandler_ParseTeams_Add(t *testing.T) {
	players := &cb.TeamsPlayersAdd{}
	players.Set([]pk.String{"Alice", "Bob"})
	flags := cb.TeamsUnnamedType0001AddFlagsBitflags{}
	flags.SetFriendlyFire(true)

	pkt := cb.NewTeams()
	pkt.Team = pk.String("RedTeam")
	pkt.Mode = cb.TeamsMode{Value: "add"}
	pkt.UnnamedType0001 = &cb.TeamsUnnamedType0001Add{
		Flags:             flags,
		NameTagVisibility: cb.TeamsUnnamedType0001AddNameTagVisibility{Value: "always"},
		CollisionRule:     cb.TeamsUnnamedType0001AddCollisionRule{Value: "always"},
	}
	pkt.Players = players

	handler := &playHandler{}
	update, err := handler.ParseTeams(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseTeams: %v", err)
	}
	if update.Team != "RedTeam" {
		t.Errorf("Team = %q, want RedTeam", update.Team)
	}
	if update.Mode != "add" {
		t.Errorf("Mode = %q, want add", update.Mode)
	}
	if !update.FriendlyFire {
		t.Error("FriendlyFire = false, want true")
	}
	if want := []string{"Alice", "Bob"}; !equalStrings(update.Players, want) {
		t.Errorf("Players = %v, want %v", update.Players, want)
	}
}

func TestPlayHandler_ParseTeams_Change(t *testing.T) {
	flags := cb.TeamsUnnamedType0001ChangeFlagsBitflags{}
	flags.SetFriendlyFire(false)

	pkt := cb.NewTeams()
	pkt.Team = pk.String("RedTeam")
	pkt.Mode = cb.TeamsMode{Value: "change"}
	pkt.UnnamedType0001 = &cb.TeamsUnnamedType0001Change{
		Flags:             flags,
		NameTagVisibility: cb.TeamsUnnamedType0001ChangeNameTagVisibility{Value: "always"},
		CollisionRule:     cb.TeamsUnnamedType0001ChangeCollisionRule{Value: "always"},
	}
	pkt.Players = &protocolmodels.Void{}

	handler := &playHandler{}
	update, err := handler.ParseTeams(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseTeams: %v", err)
	}
	if update.Mode != "change" {
		t.Errorf("Mode = %q, want change", update.Mode)
	}
	if update.FriendlyFire {
		t.Error("FriendlyFire = true, want false")
	}
}

func TestPlayHandler_ParseTeams_JoinAndLeave(t *testing.T) {
	joinPlayers := &cb.TeamsPlayersJoin{}
	joinPlayers.Set([]pk.String{"Carol"})
	joinPkt := cb.NewTeams()
	joinPkt.Team = pk.String("BlueTeam")
	joinPkt.Mode = cb.TeamsMode{Value: "join"}
	joinPkt.UnnamedType0001 = &protocolmodels.Void{}
	joinPkt.Players = joinPlayers

	handler := &playHandler{}
	update, err := handler.ParseTeams(joinPkt.Marshal())
	if err != nil {
		t.Fatalf("ParseTeams (join): %v", err)
	}
	if update.Mode != "join" || !equalStrings(update.Players, []string{"Carol"}) {
		t.Errorf("join update = %+v", update)
	}

	leavePlayers := &cb.TeamsPlayersLeave{}
	leavePlayers.Set([]pk.String{"Carol"})
	leavePkt := cb.NewTeams()
	leavePkt.Team = pk.String("BlueTeam")
	leavePkt.Mode = cb.TeamsMode{Value: "leave"}
	leavePkt.UnnamedType0001 = &protocolmodels.Void{}
	leavePkt.Players = leavePlayers

	update, err = handler.ParseTeams(leavePkt.Marshal())
	if err != nil {
		t.Fatalf("ParseTeams (leave): %v", err)
	}
	if update.Mode != "leave" || !equalStrings(update.Players, []string{"Carol"}) {
		t.Errorf("leave update = %+v", update)
	}
}

func TestPlayHandler_ParseTeams_Remove(t *testing.T) {
	pkt := cb.NewTeams()
	pkt.Team = pk.String("RedTeam")
	pkt.Mode = cb.TeamsMode{Value: "remove"}
	pkt.UnnamedType0001 = &protocolmodels.Void{}
	pkt.Players = &protocolmodels.Void{}

	handler := &playHandler{}
	update, err := handler.ParseTeams(pkt.Marshal())
	if err != nil {
		t.Fatalf("ParseTeams: %v", err)
	}
	if update.Team != "RedTeam" || update.Mode != "remove" {
		t.Errorf("update = %+v", update)
	}
}

func equalStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
