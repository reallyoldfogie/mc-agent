package activity

import (
	"github.com/spbinns/mc-agent/spbbot/types"

	go_mc_bot "github.com/Tnze/go-mc/bot"
)

type sleep struct {
	client *go_mc_bot.Client

	running bool
}

// NewSleep ...
func NewSleep(client *go_mc_bot.Client) types.Activity {
	return &sleep{
		client: client,
	}
}

func (g *sleep) RegisterCallbacks() map[string]func(params ...interface{}) error {
	return map[string]func(params ...interface{}) error{
		// "SoundPlay": g.onSound,
	}
}

func (g *sleep) GetName() string {
	return "Sleeping"
}

func (g *sleep) Start(keywords []string, command string) error {
	return nil
}

func (g *sleep) Stop() error {
	return nil
}

func (g *sleep) IsRunning() bool {
	return g.running
}
