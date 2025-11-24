package activity

import (
	"fmt"
	"log"
	"strings"

	go_mc_bot "github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/path"
	"github.com/beefsack/go-astar"
	"github.com/spbinns/mc-agent/spbbot/types"
)

type selectItem struct {
	client          *go_mc_bot.Client
	inventoryAccess types.InventoryAccess

	running             bool
	callbacksRegistered bool

	currentPath []astar.Pather
	currentStep path.Tile
}

// NewSelectItem factory
func NewSelectItem(client *go_mc_bot.Client, inventoryAccess types.InventoryAccess) types.Activity {
	return &selectItem{
		client:          client,
		inventoryAccess: inventoryAccess,
	}
}

func (g *selectItem) GetName() string {
	return "SelectItem"
}

func (g *selectItem) Start(keywords []string, command string) error {
	g.running = true
	commands := strings.Split(command, " ")
	if len(commands) > 1 {
		return g.activateItem(commands[1])
	}
	return fmt.Errorf("Invalid number of parameters")
}

func (g *selectItem) Stop() error {
	g.running = false
	return nil
}

func (g *selectItem) IsRunning() bool {
	return g.running
}

func (g *selectItem) RegisterCallbacks() map[string]func(params ...interface{}) error {
	return map[string]func(params ...interface{}) error{
		// "SoundPlay":          g.onSound,
		// "PrePhysicsCallback": g.prePhysicsCallback,
	}
}

// **** currently only works for items in the hot bar :( ****
func (g *selectItem) activateItem(item string) error {
	slotID := g.client.Player.HeldItem // the slotID for the currently held item (0-8) + 36

	slot := g.inventoryAccess.GetInventoryItem(slotID + 36)

	if !strings.Contains(strings.ToLower(slot.String()), item) {
		for i := 0; i < 9; i++ {
			tmpSlot := g.inventoryAccess.GetInventoryItem(i + 36)
			log.Printf("checking if %s is %s\n", tmpSlot.String(), item)
			if strings.Contains(strings.ToLower(tmpSlot.String()), item) {
				return g.client.SelectItem(i)
			}
		}
		newSlotID := g.client.Player.HeldItem
		newSlot := g.inventoryAccess.GetInventoryItem(newSlotID + 36)
		log.Printf("slot[%d] = %#v => slot[%d] = %#v", slotID+36, slot, newSlotID+36, newSlot)
	}
	return nil
}
