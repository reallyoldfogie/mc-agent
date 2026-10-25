package v1_21_1

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/play/clientbound"
	protocolmodels "github.com/reallyoldfogie/mc-protocol-go/models"
)

func TestPlayHandler_ParseTeams_Add(t *testing.T) {
	players := &cb.TeamsPlayers0{}
	players.Set([]pk.String{"Alice", "Bob"})
	friendlyFire := pk.Byte(1)

	pkt := cb.NewTeams()
	pkt.Team = pk.String("RedTeam")
	pkt.Mode = pk.Byte(0) // add
	pkt.Name = &protocolmodels.AnonymousNBT{}
	pkt.FriendlyFire = &friendlyFire
	nameTagVis := pk.String("")
	pkt.NameTagVisibility = &nameTagVis
	collisionRule := pk.String("")
	pkt.CollisionRule = &collisionRule
	formatting := pk.VarInt(0)
	pkt.Formatting = &formatting
	pkt.Prefix = &protocolmodels.AnonymousNBT{}
	pkt.Suffix = &protocolmodels.AnonymousNBT{}
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
	friendlyFire := pk.Byte(0)

	pkt := cb.NewTeams()
	pkt.Team = pk.String("RedTeam")
	pkt.Mode = pk.Byte(2) // change
	pkt.Name = &protocolmodels.AnonymousNBT{}
	pkt.FriendlyFire = &friendlyFire
	nameTagVis := pk.String("")
	pkt.NameTagVisibility = &nameTagVis
	collisionRule := pk.String("")
	pkt.CollisionRule = &collisionRule
	formatting := pk.VarInt(0)
	pkt.Formatting = &formatting
	pkt.Prefix = &protocolmodels.AnonymousNBT{}
	pkt.Suffix = &protocolmodels.AnonymousNBT{}
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
	joinPlayers := &cb.TeamsPlayers3{}
	joinPlayers.Set([]pk.String{"Carol"})
	joinPkt := cb.NewTeams()
	joinPkt.Team = pk.String("BlueTeam")
	joinPkt.Mode = pk.Byte(3) // join
	joinPkt.Name = &protocolmodels.Void{}
	joinPkt.FriendlyFire = &protocolmodels.Void{}
	joinPkt.NameTagVisibility = &protocolmodels.Void{}
	joinPkt.CollisionRule = &protocolmodels.Void{}
	joinPkt.Formatting = &protocolmodels.Void{}
	joinPkt.Prefix = &protocolmodels.Void{}
	joinPkt.Suffix = &protocolmodels.Void{}
	joinPkt.Players = joinPlayers

	handler := &playHandler{}
	update, err := handler.ParseTeams(joinPkt.Marshal())
	if err != nil {
		t.Fatalf("ParseTeams (join): %v", err)
	}
	if update.Mode != "join" || !equalStrings(update.Players, []string{"Carol"}) {
		t.Errorf("join update = %+v", update)
	}

	leavePlayers := &cb.TeamsPlayers4{}
	leavePlayers.Set([]pk.String{"Carol"})
	leavePkt := cb.NewTeams()
	leavePkt.Team = pk.String("BlueTeam")
	leavePkt.Mode = pk.Byte(4) // leave
	leavePkt.Name = &protocolmodels.Void{}
	leavePkt.FriendlyFire = &protocolmodels.Void{}
	leavePkt.NameTagVisibility = &protocolmodels.Void{}
	leavePkt.CollisionRule = &protocolmodels.Void{}
	leavePkt.Formatting = &protocolmodels.Void{}
	leavePkt.Prefix = &protocolmodels.Void{}
	leavePkt.Suffix = &protocolmodels.Void{}
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
	pkt.Mode = pk.Byte(1) // remove
	pkt.Name = &protocolmodels.Void{}
	pkt.FriendlyFire = &protocolmodels.Void{}
	pkt.NameTagVisibility = &protocolmodels.Void{}
	pkt.CollisionRule = &protocolmodels.Void{}
	pkt.Formatting = &protocolmodels.Void{}
	pkt.Prefix = &protocolmodels.Void{}
	pkt.Suffix = &protocolmodels.Void{}
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
