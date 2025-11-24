package spbbot

import (
	"fmt"
	"strings"

	"github.com/spbinns/mc-agent/spbbot/activity"
	"github.com/spbinns/mc-agent/spbbot/types"

	go_mc_bot "github.com/Tnze/go-mc/bot"
	world_entity "github.com/Tnze/go-mc/bot/world/entity"
)

// Bot ...
type Bot interface {
	SetClient(*go_mc_bot.Client)
	GetClient() *go_mc_bot.Client

	// Callback handlers
	PrePhysicsCallback() error
	SetActiveActivity(keywords []string) error
	GetActiveActivity() types.Activity
	ClearActiveActivity() error
	GetInventoryItem(slotID int) *world_entity.Slot
	UpdateInventory(id byte, slotID int, slot world_entity.Slot)
	OnHealthChange(oldHealth, newHealth float32, oldFood, newFood int32, oldFoodSaturation, newFoodSaturation float32) error

	GetActiveEventHandlers() map[string]func(params ...interface{}) error
}

type bot struct {
	client *go_mc_bot.Client

	keywords       map[string]string         // mapping from keywords to Goal names
	activities     map[string]types.Activity // mapping from Goal names to Goals
	activeActivity types.Activity            // the currently active Goal
	// activeActivityEventHandlers map[string]func(params ...interface{}) error
	state     State
	inventory map[int]world_entity.Slot
}

// NewBot factory
func NewBot(client *go_mc_bot.Client) Bot {
	rval := &bot{
		client: client,

		keywords: map[string]string{
			"start fishing":   "Fishing",
			"stop fishing":    "Fishing",
			"start following": "Follow",
			"stop following":  "Follow",
			"start defending": "Defend",
			"stop defending":  "Defend",
			"select":          "SelectItem",
		},
		state: newState(client.Player),
	}

	rval.activities = map[string]types.Activity{
		"Fishing":    activity.NewFishingActivity(client),
		"Follow":     activity.NewFollowPlayer(client),
		"Defend":     activity.NewDefendActivity(client, rval),
		"SelectItem": activity.NewSelectItem(client, rval),
	}

	return rval
}

func (b *bot) SetClient(c *go_mc_bot.Client) {
	if c == nil {
		panic("SetClient received a nil client!")
	}
	b.client = c
}

func (b *bot) GetClient() *go_mc_bot.Client {
	return b.client
}

func (b *bot) GetActiveEventHandlers() map[string]func(params ...interface{}) error {
	if b.activeActivity != nil {
		return b.activeActivity.RegisterCallbacks()
	}
	return nil
	// return b.activeActivityEventHandlers
}

func (b *bot) PrePhysicsCallback() error {
	return nil
}

func (b *bot) SetActiveActivity(keywords []string) error {
	var prevKeyword string
	for _, keyword := range keywords {
		if strings.ToLower(keyword) == "start" || strings.ToLower(keyword) == "stop" || strings.ToLower(keyword) == "select" {
			prevKeyword = strings.ToLower(keyword)
		} else if prevKeyword != "" {
			if prevKeyword == "start" {
				command := fmt.Sprintf("%s %s", prevKeyword, keyword)
				goal, ok := b.activities[b.keywords[command]]
				if !ok {
					return fmt.Errorf("[start] %s is an invalid command", command)
				}
				err := goal.Start(keywords, command)
				if err != nil {
					return err
				}
				if goal.IsRunning() == true {
					b.activeActivity = goal
					return nil
				}
				return fmt.Errorf("'%s' failed to start", command)
			} else if prevKeyword == "select" {
				command := fmt.Sprintf("%s %s", prevKeyword, keyword)
				goal, ok := b.activities["SelectItem"]
				if !ok {
					return fmt.Errorf("[select] %s is an invalid command", command)
				}
				err := goal.Start(keywords, command)
				if err != nil {
					return err
				}

				if goal.IsRunning() == true {
					defer goal.Stop()
					return nil
				}
			}
		}
	}
	return fmt.Errorf("goal not found for command: %v", keywords)
}

func (b *bot) GetActiveActivity() types.Activity {
	return b.activeActivity
}

func (b *bot) ClearActiveActivity() (err error) {
	if b.activeActivity != nil {
		err = b.activeActivity.Stop()
		b.activeActivity = nil
	}
	return err
}

func (b *bot) UpdateInventory(id byte, slotID int, slot world_entity.Slot) {
	fmt.Printf("Updating inventory: (id: %v, slotID: %d, slot: %v)\n", id, slotID, slot)
	b.state.Inventory[slotID] = slot
}

func (b *bot) GetInventoryItem(slotID int) *world_entity.Slot {
	if slot, ok := b.state.Inventory[slotID]; ok {
		return &slot
	}
	return nil
}

func (b *bot) OnHealthChange(oldHealth, newHealth float32, oldFood, newFood int32, oldFoodSaturation, newFoodSaturation float32) error {
	fmt.Printf("\foldHealth: %f\nnewHealth: %f\noldFood:%d\nnewFood:%d\noldFoodSaturation:%f\nnewFoodSaturation:%f\n\n",
		oldHealth, newHealth, oldFood, newFood, oldFoodSaturation, newFoodSaturation)

	return nil
}
