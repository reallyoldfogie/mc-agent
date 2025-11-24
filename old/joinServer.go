package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	go_mc_bot "github.com/Tnze/go-mc/bot"
	world_entity "github.com/Tnze/go-mc/bot/world/entity"
	"github.com/Tnze/go-mc/bot/world/entity/player"
	"github.com/Tnze/go-mc/chat"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil"
	"github.com/google/uuid"

	"github.com/spbinns/mc-agent/spbbot"
)

type authCreds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var creds *authCreds

func connect(credentials *authCreds) (*go_mc_bot.Client, error) {
	c := go_mc_bot.NewClient()

	//Login Mojang account to get AccessToken
	auth, err := yggdrasil.Authenticate(credentials.Username, credentials.Password)
	if err != nil {
		// panic(err)
		return nil, fmt.Errorf("Failed to authenticate: %v", err)
	}

	c.Auth.UUID, c.Name = auth.SelectedProfile()
	c.AsTk = auth.AccessToken()

	//Connect server
	err = c.JoinServer("localhost", 25565)
	if err != nil {
		return nil, fmt.Errorf("Failed to join server: %v", err)
	}
	log.Println("Login success")

	return c, nil
}

func loadCredentials(fileName string) (*authCreds, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("Failed to open %s: %v", fileName, err)
	}

	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	data := authCreds{}

	err = json.Unmarshal(contents, &data)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal auth credentials: %v", err)
	}

	return &data, nil
}

// JoinServer example
func JoinServer() {
	var err error
	creds, err = loadCredentials("rof_botCreds.json")
	// creds, err := loadCredentials("reallyOldFogieCreds.json")
	if err != nil {
		log.Fatal(err)
	}

	c, err := connect(creds)
	if err != nil {
		log.Fatal(err)
	}

	bot := spbbot.NewBot(c)

	// Regist event handlers
	// 	c.Events.GameStart = onGameStartFunc
	// 	c.Events.ChatMsg = onChatMsgFunc
	// 	c.Events.Disconnect = onDisconnectFunc
	//	...

	c.Events.GameStart = onGameStartFunc
	c.Events.Disconnect = onGameDisconnectFunc(bot)
	c.Events.Die = onGameDie(c, bot)
	c.Events.ChatMsg = onChatMsgFunc(c, bot)
	c.Events.PositionChange = onPositionChange(c)
	c.Events.SoundPlay = onSoundPlay(bot)
	c.Events.HealthChange = onHealthChange(bot)

	// items
	c.Events.WindowsItemChange = onWindowsItemChange(bot)
	c.Events.WindowsItem = onWindowsItem(bot)
	c.Events.PrePhysics = onPrePhysicsFunc(bot)

	// c.Events.PrePhysics = bot.PrePhysicsCallback

	c.Events.GameReady = onGameReady(c, bot)
	c.Events.ReceivePacket = onReceivePacket

	// tell the physics engine to run
	c.Physics.Run = true

	//Join the game
	err = c.HandleGame()
	if err != nil {
		log.Println("[ERROR]", err)
		log.Fatal(err)
	}
}

func onGameStartFunc() error {
	fmt.Println("Game started")
	return nil
}

func onGameDisconnectFunc(bot spbbot.Bot) func(reason chat.Message) error {
	return func(reason chat.Message) error {

		fmt.Printf("disconnected: %s\n", reason.ClearString())

		time.Sleep(5 * time.Second)
		fmt.Printf("trying to reconnect")

		// var err error
		c, err := connect(creds)
		if err != nil {

		}

		bot.SetClient(c)
		return nil
	}
}

func onGameDie(c *go_mc_bot.Client, bot spbbot.Bot) func() error {
	return func() error {
		fmt.Println("Died!")
		// bot.ClearActiveActivity()
		return c.Respawn()
	}
}

func onChatMsgFunc(client *go_mc_bot.Client, bot spbbot.Bot) func(msg chat.Message, pos byte, sender uuid.UUID) error {
	return func(msg chat.Message, pos byte, sender uuid.UUID) error {
		str := msg.String()
		if strings.Contains(str, "EXTRA") {
			fmt.Printf("unknown string key: %s \n", msg.Translate)
		}
		fmt.Printf("chat received: %s\n", str)

		plainMsg := strings.ToLower(msg.ClearString())
		if strings.Contains(plainMsg, "start") || strings.Contains(plainMsg, "stop") || strings.Contains(plainMsg, "select") {
			commands := strings.Split(plainMsg, " ")
			if strings.Contains(plainMsg, "start") || strings.Contains(plainMsg, "select") {
				err := bot.SetActiveActivity(commands)
				if err != nil {
					log.Println("Invalid command:", commands, err)
				}
				return nil
			}
			return bot.ClearActiveActivity()

		}

		if strings.Contains(plainMsg, "what") {
			if strings.Contains(plainMsg, "doing") {
				goal := bot.GetActiveActivity()
				action := "doing nothing"
				if goal != nil {
					action = strings.ToLower(goal.GetName())
				}
				return client.Chat(fmt.Sprintf("I am %s", action))
			}

			if strings.Contains(plainMsg, "holding") {
				slotID := client.Player.HeldItem

				slot := bot.GetInventoryItem(slotID + 36)
				log.Println(slotID, slotID+36, slot)
				if slot != nil {
					return client.Chat(fmt.Sprintf("I am holding a %s", strings.ToLower(slot.String())))
				}
				return client.Chat("I am not holding anything, or I am holding an unknown item")
			}

		}
		return nil
	}
}

func onPositionChange(c *go_mc_bot.Client) func(pos player.Pos) error {
	return func(pos player.Pos) error {
		fmt.Printf("Player position changed: %+v\n", pos)
		return nil
	}
}

func onSoundPlay(bot spbbot.Bot) func(name string, category int, x, y, z float64, volume, pitch float32) error {
	return func(name string, category int, x, y, z float64, volume, pitch float32) error {
		// log.Println("SoundPlay event received [", name, "]")
		eventHandlers := bot.GetActiveEventHandlers()
		if eventHandlers != nil {
			if handler, ok := eventHandlers["SoundPlay"]; ok {
				// log.Println("dispatching SoundPlay event to active goal (", name, ")")
				params := make([]interface{}, 7)
				params[0] = name
				params[1] = category
				params[2] = x
				params[3] = y
				params[4] = z
				params[5] = volume
				params[6] = pitch

				return handler(params...)
			}
			// log.Println("active goal doesn't handle SoundPlay event")
		}

		activityName := "unknown"
		currentActivity := bot.GetActiveActivity()
		if currentActivity != nil {
			activityName = currentActivity.GetName()
		}
		log.Printf("active goal (%s) doesn't handle any sound events\n", activityName)
		log.Printf("Unhandled sound: %s (%f, %f, %f)\n", name, x, y, z)
		log.Printf("Player          Pos: %f, %f, %f\n", bot.GetClient().Player.Pos.X, bot.GetClient().Player.Pos.Y, bot.GetClient().Player.Pos.Z)

		return nil
	}
}

func onHealthChange(bot spbbot.Bot) func(oldHealth, newHealth float32, oldFood, newFood int32, oldFoodSaturation, newFoodSaturation float32) error {
	return bot.OnHealthChange
}
func onPrePhysicsFunc(bot spbbot.Bot) func() error {
	return func() error {
		eventHandlers := bot.GetActiveEventHandlers()
		if eventHandlers != nil {
			if handler, ok := eventHandlers["PrePhysicsCallback"]; ok {
				// log.Println("dispatching SoundPlay event to active goal (", name, ")")
				params := []interface{}{}

				return handler(params...)
			}
			// log.Println("active goal doesn't handle SoundPlay event")
		}
		// log.Println("active goal doesn't handle any events")

		return nil
	}
}

func onWindowsItemChange(bot spbbot.Bot) func(id byte, slotID int, slot world_entity.Slot) error {
	return func(id byte, slotID int, slot world_entity.Slot) error {
		fmt.Printf("WindowsItemChange: id: %v, slotID: %v, slot: %v\n", id, slotID, slot)
		bot.UpdateInventory(id, slotID, slot)
		return nil
	}
}

func onWindowsItem(bot spbbot.Bot) func(id byte, slots []world_entity.Slot) error {
	return func(id byte, slots []world_entity.Slot) error {
		fmt.Printf("WindowsItem: id: %v, slots: %v\n", id, slots)
		return nil
	}
}

var gameReady bool

func onReceivePacket(p pk.Packet) (pass bool, err error) {
	// switch p.ID {
	// case int32(data.Map):
	// 	fallthrough
	// case int32(data.EntityHeadRotation):
	// 	fallthrough
	// case int32(data.EntityVelocity):
	// 	fallthrough
	// case int32(data.EntityMoveLook):
	// 	fallthrough
	// case int32(data.EntityMetadata):
	// 	fallthrough
	// case int32(data.RelEntityMove):
	// 	fallthrough
	// case int32(data.EntityStatus):
	// 	fallthrough
	// case int32(data.EntityLook):
	// 	fallthrough
	// case int32(data.SoundEffect):
	// 	fallthrough
	// case int32(data.Success):
	// 	fallthrough
	// case int32(data.EntityEquipment):
	// 	fallthrough

	// case int32(data.EntityTeleport):
	// 	return false, nil
	// }
	// fmt.Println("packet:", p)

	// fmt.Printf("Packet receive => 0x%x:\t%s\n", p.ID, data.PktID(p.ID).String() /*, string(p.Data)*/)
	return false, nil
}

func onGameReady(c *go_mc_bot.Client, bot spbbot.Bot) func() error {
	return func() error {
		gameReady = true
		fmt.Println("Game Ready")

		bot.SetActiveActivity([]string{"start", "defending"})
		return nil
	}
}
