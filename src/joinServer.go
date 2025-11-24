package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	mc_bot "github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/net/ptypes"
	"github.com/google/uuid"
	msauth "github.com/maxsupermanhd/go-mc-ms-auth"
)

type authCreds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

const timeout = 45

var (
	creds *authCreds
	p     *basic.Player
	c     *mc_bot.Client

	heldItem int

	// inventoryAccess types.InventoryAccess

	watch chan time.Time
)

func JoinServer(addr string, credfile string) {
	var err error
	creds, err = loadCredFile(credfile)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Logging in as %s\n", creds.Username)
	c, err = connect(addr, creds)
	if err != nil {
		log.Fatal(err)
	}

	// Register event handlers
	// 	c.Events.GameStart = onGameStartFunc
	// 	c.Events.ChatMsg = onChatMsgFunc
	// 	c.Events.Disconnect = onDisconnectFunc
	//	...

	//Join the game
	err = c.HandleGame()
	if err != nil {
		log.Println("[ERROR]", err)
		log.Fatal(err)
	}
}

func connect(addr string, credentials *authCreds) (*mc_bot.Client, error) {
	c := mc_bot.NewClient()
	p = basic.NewPlayer(c, basic.DefaultSettings, basic.EventsListener{
		GameStart: onGameStart,
		// ChatMsg:    onChatMsg,
		Disconnect: onDisconnect,
		Death:      onDeath,
	})

	//Register event handlers
	// basic.EventsListener{
	// 	GameStart:  onGameStart,
	// 	ChatMsg:    onChatMsg,
	// 	Disconnect: onDisconnect,
	// 	Death:      onDeath,
	// }.Attach(c)
	c.Events.AddListener([]mc_bot.PacketHandler{soundListener, onChatMsg})

	//Login Mojang account to get AccessToken
	// auth, err := yggdrasil.Authenticate(credentials.Username, credentials.Password)
	// if err != nil {
	// 	// panic(err)
	// 	return nil, fmt.Errorf("Failed to authenticate: %v", err)
	// }

	// c.Auth.UUID, c.Auth.Name = auth.SelectedProfile()
	// c.Auth.AsTk = auth.AccessToken()

	// ms-auth
	mauth, err := msauth.GetMCcredentials(credentials.Username, credentials.Password)
	if err != nil {
		log.Print(err)
		panic(err)
	}
	log.Print("Authenticated as ", mauth.Name, " (", mauth.UUID, ")")
	c.Auth = mauth
	// client can go brrr

	//Connect server
	err = c.JoinServer(addr)
	if err != nil {
		return nil, fmt.Errorf("Failed to join server(%v [%v,%v {%v}]): %v", addr, c.Auth.UUID, c.Auth.Name, c.Name, err)
	}
	log.Println("Login success")

	return c, nil
}

func loadCredFile(fileName string) (*authCreds, error) {
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

var (
	soundListener = mc_bot.PacketHandler{
		ID:       packetid.ClientboundSound,
		Priority: 0,
		F: func(p pk.Packet) error {
			var (
				SoundName     pk.Identifier
				SoundCategory pk.VarInt
				X, Y, Z       pk.Int
				Volume, Pitch pk.Float
			)
			if err := p.Scan(&SoundName, &SoundCategory, &X, &Y, &Z, &Volume, &Pitch); err != nil {
				return err
			}
			return onSound(string(SoundName), int(SoundCategory), float64(X)/8, float64(Y)/8, float64(Z)/8, float32(Volume), float32(Pitch))
		},
	}

	chatListener = mc_bot.PacketHandler{
		ID:       packetid.ClientboundPlayerChat,
		Priority: 0,
		F: func(p pk.Packet) error {
			var (
				c          chat.Message
				pos        byte
				playerUuid uuid.UUID
			)
			if err := p.Scan(); err != nil {
				return err
			}
			return onChatMsg(c, pos, playerUuid)
		},
	}
)

var getActiveItem = mc_bot.PacketHandler{
	ID:       packetid.ClientboundHeldItemSlot, // HeldItemSlotClientbound,
	Priority: 0,
	F: func(pkt pk.Packet) error {
		return handleHeldItemPacket(c, pkt)
	},
}

func handleHeldItemPacket(c *mc_bot.Client, pkt pk.Packet) error {
	var hi pk.Byte
	if err := pkt.Scan(&hi); err != nil {
		return err
	}
	heldItem = int(hi)

	fmt.Printf("received HeldItem packet: %v\n", heldItem)

	// if c.Events.HeldItemChange != nil {
	// 	return c.Events.HeldItemChange(c.HeldItem)
	// }
	return nil
}

func UseItem(hand int32) error {
	return c.Conn.WritePacket(pk.Marshal(
		packetid.ServerboundUseItem,
		pk.VarInt(hand),
	))
}

func onDeath() error {
	log.Println("Died and Respawned")
	// If we exclude Respawn(...) then the player won't press the "Respawn" button upon death
	return p.Respawn()
}

func onGameStart() error {
	log.Println("Game start")

	watch = make(chan time.Time)
	go watchDog()

	fmt.Println("held item: ", heldItem)
	log.Println("Starting fishing")

	return UseItem(0)
}

// SelectItem used to change the slot selection in hotbar.
// slot should from 0 to 8
func SelectItem(slot int) error {
	if slot < 0 || slot > 8 {
		return errors.New("invalid slot: " + strconv.Itoa(slot))
	}

	return c.Conn.WritePacket(pk.Marshal(
		packetid.ServerboundHeldItemSlot,
		pk.Short(slot),
	))
}

//goland:noinspection SpellCheckingInspection
func onSound(name string, category int, x, y, z float64, volume, pitch float32) error {
	if name == "entity.fishing_bobber.splash" {
		if err := UseItem(0); err != nil { //retrieve
			return err
		}
		log.Println("gra~")
		time.Sleep(time.Millisecond * 300)
		if err := UseItem(0); err != nil { //throw
			return err
		}
		watch <- time.Now()
	}
	return nil
}

func onChatMsg(c chat.Message, pos byte, uuid uuid.UUID) error {
	log.Println("Chat:", c)
	return nil
}

func onDisconnect(c chat.Message) error {
	log.Println("Disconnect:", c)
	return nil
}

func watchDog() {
	to := time.NewTimer(time.Second * timeout)
	for {
		select {
		case <-watch:
		case <-to.C:
			log.Println("rethrow")
			if err := UseItem(0); err != nil {
				panic(err)
			}
		}
		to.Reset(time.Second * timeout)
	}
}

// PickItem used to swap out an empty space on the hotbar with the item in the given inventory slot.
// The Notchain client uses this for pick block functionality (middle click) to retrieve items from the inventory.
//
// The server will first search the player's hotbar for an empty slot,
// starting from the current slot and looping around to the slot before it.
// If there are no empty slots, it will start a second search from the
// current slot and find the first slot that does not contain an enchanted item.
// If there still are no slots that meet that criteria, then the server will
// use the currently selected slot. After finding the appropriate slot,
// the server swaps the items and then change player's selected slot (cause the HeldItemChange event).
func PickItem(slot int) error {
	return c.Conn.WritePacket(pk.Marshal(
		packetid.ServerboundPickItem,
		pk.VarInt(slot),
	))
}

func ActivateItem(item string) error {
	// slotID := heldItem // the slotID for the currently held item (0-8) + 36

	// slot := inventoryAccess.GetInventoryItem(slotID + 36)

	// if !strings.Contains(strings.ToLower(slot.String()), item) {
	// 	for i := 0; i < 9; i++ {
	// 		tmpSlot := g.inventoryAccess.GetInventoryItem(i + 36)
	// 		log.Printf("checking if %s is %s\n", tmpSlot.String(), item)
	// 		if strings.Contains(strings.ToLower(tmpSlot.String()), item) {
	// 			return SelectItem(i)
	// 		}
	// 	}
	// 	newSlotID := g.client.Player.HeldItem
	// 	newSlot := g.inventoryAccess.GetInventoryItem(newSlotID + 36)
	// 	log.Printf("slot[%d] = %#v => slot[%d] = %#v", slotID+36, slot, newSlotID+36, newSlot)
	// }
	return nil
}

func handleWindowItemsPacket(c *mc_bot.Client, p pk.Packet) error {
	var pkt ptypes.WindowItems
	if err := pkt.Decode(p); err != nil {
		return err
	}
	fmt.Printf("WindowItems: %#v\n", pkt)

	// if pkt.WindowID == 0 { // Window ID 0 is the players' inventory.
	// 	if err := c.Events.updateSeenPackets(seenPlayerInventory); err != nil {
	// 		return err
	// 	}
	// }
	// if c.Events.WindowsItem != nil {
	// 	return c.Events.WindowsItem(byte(pkt.WindowID), pkt.Slots)
	// }
	return nil
}

func handleOpenWindowPacket(c *mc_bot.Client, p pk.Packet) error {
	var pkt ptypes.OpenWindow
	if err := pkt.Decode(p); err != nil {
		return err
	}
	fmt.Printf("OpenWindow: %#v\n", pkt)

	// if c.Events.OpenWindow != nil {
	// 	return c.Events.OpenWindow(pkt)
	// }
	return nil
}

func handleWindowConfirmationPacket(c *mc_bot.Client, p pk.Packet) error {
	var pkt ptypes.ConfirmTransaction
	if err := pkt.Decode(p); err != nil {
		return err
	}

	fmt.Printf("WindowConfirmation: %#v\n", pkt)

	// if c.Events.WindowConfirmation != nil {
	// 	return c.Events.WindowConfirmation(pkt)
	// }
	return nil
}

// func handleSetExperience(c *mc_bot.Client, p pk.Packet) (err error) {
// 	var (
// 		bar   pk.Float
// 		level pk.VarInt
// 		total pk.VarInt
// 	)

// 	if err := p.Scan(&bar, &level, &total); err != nil {
// 		return err
// 	}

// 	c.Level = int32(level)

// 	if c.Events.ExperienceChange != nil {
// 		return c.Events.ExperienceChange(float32(bar), int32(level), int32(total))
// 	}

// 	return nil
// }

// func UpdateInventory(id byte, slotID int, slot world_entity.Slot) {
// 	fmt.Printf("Updating inventory: (id: %v, slotID: %d, slot: %v)\n", id, slotID, slot)
// 	b.state.Inventory[slotID] = slot
// }

// func GetInventoryItem(slotID int) *world_entity.Slot {
// 	if slot, ok := b.state.Inventory[slotID]; ok {
// 		return &slot
// 	}
// 	return nil
// }

func OnHealthChange(oldHealth, newHealth float32, oldFood, newFood int32, oldFoodSaturation, newFoodSaturation float32) error {
	fmt.Printf("\foldHealth: %f\nnewHealth: %f\noldFood:%d\nnewFood:%d\noldFoodSaturation:%f\nnewFoodSaturation:%f\n\n",
		oldHealth, newHealth, oldFood, newFood, oldFoodSaturation, newFoodSaturation)

	return nil
}
