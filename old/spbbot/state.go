package spbbot

import (
	world_entity "github.com/Tnze/go-mc/bot/world/entity"
	"github.com/Tnze/go-mc/bot/world/entity/player"
)

// State ...
type State struct {
	// CurrentHealth float32 // in player
	Inventory map[int]world_entity.Slot

	OffHandSelection  int64
	MainHandSelection int64

	VisibleItems []interface{}

	Player    player.Player
	Dimension string
}

func newState(player player.Player) State {
	return State{
		Player:       player,
		Inventory:    map[int]world_entity.Slot{},
		VisibleItems: []interface{}{},
	}
}
