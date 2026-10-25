package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/reallyoldfogie/mc-agent/models"

	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
)

func TestApplyTeamsUpdate_AddJoinLeaveRemove(t *testing.T) {
	teams := make(map[string]*teamInfo)

	applyTeamsUpdate(teams, &models.TeamsUpdate{Team: "Red", Mode: "add", FriendlyFire: true, Players: []string{"Alice", "Bob"}})
	team, ok := teams["Red"]
	if !ok {
		t.Fatal("Red team not created by add")
	}
	if !team.friendlyFire {
		t.Error("friendlyFire = false, want true")
	}
	if !team.members["Alice"] || !team.members["Bob"] {
		t.Errorf("members = %v, want Alice and Bob", team.members)
	}

	applyTeamsUpdate(teams, &models.TeamsUpdate{Team: "Red", Mode: "join", Players: []string{"Carol"}})
	if !teams["Red"].members["Carol"] {
		t.Error("join did not add Carol to Red")
	}

	applyTeamsUpdate(teams, &models.TeamsUpdate{Team: "Red", Mode: "leave", Players: []string{"Bob"}})
	if teams["Red"].members["Bob"] {
		t.Error("leave did not remove Bob from Red")
	}
	if !teams["Red"].members["Alice"] {
		t.Error("leave incorrectly removed an uninvolved member")
	}

	applyTeamsUpdate(teams, &models.TeamsUpdate{Team: "Red", Mode: "change", FriendlyFire: false})
	if teams["Red"].friendlyFire {
		t.Error("change did not update friendlyFire")
	}

	applyTeamsUpdate(teams, &models.TeamsUpdate{Team: "Red", Mode: "remove"})
	if _, ok := teams["Red"]; ok {
		t.Error("remove did not delete the Red team")
	}
}

func TestApplyTeamsUpdate_JoinCreatesTeamIfMissing(t *testing.T) {
	teams := make(map[string]*teamInfo)
	applyTeamsUpdate(teams, &models.TeamsUpdate{Team: "Blue", Mode: "join", Players: []string{"Dave"}})
	team, ok := teams["Blue"]
	if !ok {
		t.Fatal("join should create a team that doesn't yet exist")
	}
	if !team.members["Dave"] {
		t.Error("Dave not added to newly created Blue team")
	}
}

func TestTeamForPlayerName(t *testing.T) {
	a := &agent{teams: map[string]*teamInfo{
		"Red":  {members: map[string]bool{"Alice": true}},
		"Blue": {members: map[string]bool{"Dave": true}},
	}}
	if got := a.teamForPlayerName("Alice"); got != "Red" {
		t.Errorf("teamForPlayerName(Alice) = %q, want Red", got)
	}
	if got := a.teamForPlayerName("Nobody"); got != "" {
		t.Errorf("teamForPlayerName(Nobody) = %q, want empty (free-for-all)", got)
	}
}

// fakePlayerList is a minimal playerlist.PlayerList for bridging a tracked
// entity's UUID to a player name without a live server.
type fakePlayerList struct {
	infos map[uuid.UUID]*playerlist.PlayerInfo
}

func (f *fakePlayerList) Get() map[uuid.UUID]*playerlist.PlayerInfo { return f.infos }
func (f *fakePlayerList) Set(infos map[uuid.UUID]*playerlist.PlayerInfo) {
	f.infos = infos
}
func (f *fakePlayerList) GetPlayerInfo(id uuid.UUID) (*playerlist.PlayerInfo, bool) {
	info, ok := f.infos[id]
	return info, ok
}

func TestTeamForPlayerEntityID(t *testing.T) {
	playerUUID := uuid.New()
	a := &agent{
		entities: map[int32]*trackedEntity{
			42: {EntityID: 42, UUID: playerUUID},
		},
		playerList: &fakePlayerList{infos: map[uuid.UUID]*playerlist.PlayerInfo{
			playerUUID: {GameProfile: playerlist.GameProfile{ID: playerUUID, Name: "Alice"}},
		}},
		teams: map[string]*teamInfo{
			"Red": {members: map[string]bool{"Alice": true}},
		},
	}

	if got := a.teamForPlayerEntityID(42); got != "Red" {
		t.Errorf("teamForPlayerEntityID(42) = %q, want Red", got)
	}
	if got := a.teamForPlayerEntityID(999); got != "" {
		t.Errorf("teamForPlayerEntityID(unknown entity) = %q, want empty", got)
	}
}

// teamRCON answers "data get entity <name> Pos" for any of several named
// players and records every command issued, so SeedMatchTeam's own
// proximity-gating and team-command sequence can be exercised without a
// live server.
type teamRCON struct {
	execOnlyRCON
	positions map[string][3]float64
}

func (r *teamRCON) Exec(ctx context.Context, cmd string) (string, error) {
	r.cmds = append(r.cmds, cmd)
	for name, pos := range r.positions {
		if cmd == fmt.Sprintf("data get entity %s Pos", name) {
			return fmt.Sprintf("%s has the following entity data: [%.1fd, %.1fd, %.1fd]", name, pos[0], pos[1], pos[2]), nil
		}
	}
	return "", nil
}

func newTeamRCONTestAgent(t *testing.T, rc *teamRCON) *agent {
	t.Helper()
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565", Name: "TestBot", RCON: rc})
	if err != nil {
		t.Fatal(err)
	}
	return agentInt.(*agent)
}

func TestSeedMatchTeam_RefusesWhenAnyMemberIsFarFromArena(t *testing.T) {
	rc := &teamRCON{positions: map[string][3]float64{
		"Near": {1, 64, 1},
		"Far":  {500, 64, 500},
	}}
	a := newTeamRCONTestAgent(t, rc)
	err := a.SeedMatchTeam(context.Background(), "Red", []string{"Near", "Far"}, models.V3{X: 0, Y: 64, Z: 0}, 10)
	if err == nil {
		t.Fatal("want an error when any member is far from the arena")
	}
	for _, cmd := range rc.cmds {
		if cmd == "team add Red" {
			t.Fatalf("SeedMatchTeam must not touch team state when any member fails the proximity check, but sent: %q", cmd)
		}
	}
}

func TestSeedMatchTeam_CreatesTeamAndAddsMembersWhenAllNear(t *testing.T) {
	rc := &teamRCON{positions: map[string][3]float64{
		"Alice": {1, 64, 1},
		"Bob":   {2, 64, 2},
	}}
	a := newTeamRCONTestAgent(t, rc)
	if err := a.SeedMatchTeam(context.Background(), "Red", []string{"Alice", "Bob"}, models.V3{X: 0, Y: 64, Z: 0}, 10); err != nil {
		t.Fatalf("SeedMatchTeam: %v", err)
	}

	want := []string{"team add Red", "team modify Red friendlyFire false", "team join Red Alice", "team join Red Bob"}
	for _, wantCmd := range want {
		found := false
		for _, cmd := range rc.cmds {
			if cmd == wantCmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("commands = %v, want to include %q", rc.cmds, wantCmd)
		}
	}
}
