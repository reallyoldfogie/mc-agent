package agent

import (
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
	"github.com/reallyoldfogie/mc-agent/models"
)

// teamInfo tracks one vanilla scoreboard team's current membership (player
// names, as the Teams packet itself identifies members) and friendly-fire
// setting. docs/plans/COMBAT_COMPETITION_PLAN.md Phase 0 item 4.
type teamInfo struct {
	friendlyFire bool
	members      map[string]bool
}

// onTeams updates a.teams from a Teams (scoreboard) packet - registered
// against the real clientbound packet ID in a.handlers(), the same
// pattern every other packet handler in this file's sibling
// agent/handlers.go already uses. The actual add/change/join/leave/remove
// state transitions live in applyTeamsUpdate below so they're unit-testable
// without a real version handler or packet bytes.
func (a *agent) onTeams(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}
	update, err := a.versionHandler.Play().ParseTeams(p)
	if err != nil {
		return err
	}

	a.teamsMu.Lock()
	defer a.teamsMu.Unlock()
	if a.teams == nil {
		a.teams = make(map[string]*teamInfo)
	}
	applyTeamsUpdate(a.teams, update)
	return nil
}

// applyTeamsUpdate mutates teams in place to reflect a single Teams
// (scoreboard) packet update. See models.TeamsUpdate's own doc comment for
// the mode semantics ("add"/"remove"/"change"/"join"/"leave").
func applyTeamsUpdate(teams map[string]*teamInfo, update *models.TeamsUpdate) {
	switch update.Mode {
	case "add":
		team := &teamInfo{friendlyFire: update.FriendlyFire, members: make(map[string]bool, len(update.Players))}
		for _, name := range update.Players {
			team.members[name] = true
		}
		teams[update.Team] = team
	case "remove":
		delete(teams, update.Team)
	case "change":
		if team, ok := teams[update.Team]; ok {
			team.friendlyFire = update.FriendlyFire
		}
	case "join":
		team, ok := teams[update.Team]
		if !ok {
			team = &teamInfo{members: make(map[string]bool, len(update.Players))}
			teams[update.Team] = team
		}
		for _, name := range update.Players {
			team.members[name] = true
		}
	case "leave":
		if team, ok := teams[update.Team]; ok {
			for _, name := range update.Players {
				delete(team.members, name)
			}
		}
	}
}

// teamForPlayerName returns the scoreboard team name a player belongs to,
// or "" if they're on no team (free-for-all). combat.Target.Team and
// combat.TargetPolicy.OwnTeam (agent/combat_targets.go) both key on this.
func (a *agent) teamForPlayerName(name string) string {
	a.teamsMu.RLock()
	defer a.teamsMu.RUnlock()
	for teamName, team := range a.teams {
		if team.members[name] {
			return teamName
		}
	}
	return ""
}

// teamForPlayerEntityID resolves a tracked player entity's current
// scoreboard team by cross-referencing its spawn UUID (trackedEntity.UUID)
// against the player list's own UUID-keyed name lookup - the Teams packet
// identifies members by player name, while tracked entities are keyed by
// entity ID, so this bridges the two. Returns "" (no team) if the entity
// isn't a currently-known player, the same as an actual no-team result.
func (a *agent) teamForPlayerEntityID(entityID int32) string {
	a.entitiesMu.RLock()
	entity, ok := a.entities[entityID]
	a.entitiesMu.RUnlock()
	if !ok || a.playerList == nil {
		return ""
	}
	info, found := a.playerList.GetPlayerInfo(uuid.UUID(entity.UUID))
	if !found {
		return ""
	}
	return a.teamForPlayerName(info.Name)
}
