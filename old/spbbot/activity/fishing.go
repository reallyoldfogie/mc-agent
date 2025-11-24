package activity

import (
	"fmt"
	"log"
	"time"

	go_mc_bot "github.com/Tnze/go-mc/bot"

	"github.com/spbinns/mc-agent/spbbot/types"
)

const fishingTimeout = 45

type fishing struct {
	client *go_mc_bot.Client
	watch  chan time.Time
	quit   chan bool

	running bool
}

// NewFishingActivity -
func NewFishingActivity(client *go_mc_bot.Client) types.Activity {
	return &fishing{
		client: client,
		watch:  make(chan time.Time),
	}
}

func (g *fishing) GetName() string {
	return "Fishing"
}

func (g *fishing) Start(keywords []string, command string) error {
	// TO DO: Make sure the active item is a fishing_rod, for now just assume it is the Selected item
	fmt.Println("held item: ", g.client.HeldItem)
	log.Println("Starting fishing")
	g.quit = make(chan bool)
	go g.watchDog(g.quit)

	g.running = true
	return g.client.UseItem(0)
}

func (g *fishing) Stop() error {
	log.Println("Stopping fishing")

	g.running = false

	close(g.quit)
	return g.client.UseItem(0)
}

func (g *fishing) RegisterCallbacks() map[string]func(params ...interface{}) error {
	return map[string]func(params ...interface{}) error{
		"SoundPlay": g.onSound,
	}
}

func (g *fishing) IsRunning() bool {
	return g.running
}

// onSound(name string, category int, x, y, z float64, volume, pitch float32)
func (g *fishing) onSound(params ...interface{}) error {
	// log.Println("Fishing goal handling sound:", params)
	var name string
	// var category int
	// var x, y, z float64
	// var volume, pitch float32

	if len(params) != 7 {
		return fmt.Errorf("Invalid number of parameters to fishing.onSound. got %d, expected 7", len(params))
	}

	name = params[0].(string)
	// category = params[1].(int)
	// x = params[2].(float64)
	// y = params[3].(float64)
	// z = params[4].(float64)
	// volume = params[5].(float32)
	// pitch = params[6].(float32)

	if name == "entity.fishing_bobber.splash" {
		if err := g.client.UseItem(0); err != nil { //retrieve
			return err
		}
		log.Println("gra~")
		time.Sleep(time.Millisecond * 300)
		if err := g.client.UseItem(0); err != nil { //throw
			return err
		}
		g.watch <- time.Now()
	}
	if name == "entity.item.break" {
		if g.client.HeldItem == 0 {
			log.Println("fishing rod broke!")
			g.Stop()
		}
	}
	return nil
}

func (g *fishing) watchDog(quit chan bool) {
	to := time.NewTimer(time.Second * fishingTimeout)
	for {
		select {
		case <-quit:
			log.Println("Stopping fishing watchdog")
			return
		case <-g.watch:
		case <-to.C:
			log.Println("rethrow")
			if err := g.client.UseItem(0); err != nil {
				panic(err)
			}
		}
		to.Reset(time.Second * fishingTimeout)
	}
}
