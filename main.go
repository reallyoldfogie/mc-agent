// Daze is a basic bot example that join a server as client with these features:
// - Say "Hello, world" when game started.
// - Just standing there and do nothing.
// - Receive chat message and print it.
// - Receive player list and store it in memory.
// - Receive chunk data and load/unload them.
// - Receive inventory items and print them.
// - Automatically reborn after 5 seconds of death.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	//"github.com/mattn/go-colorable"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/registryid"
	"github.com/Tnze/go-mc/level"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/davecgh/go-spew/spew"

	"github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/basic"
	"github.com/reallyoldfogie/mc-bot-go/bot/msg"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-bot-go/bot/world"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"

	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"

	"github.com/reallyoldfogie/mc-agent/movement"

	msauth "github.com/maxsupermanhd/go-mc-ms-auth"
	"gopkg.in/natefinch/lumberjack.v2"
)

// type (
// 	authCreds struct {
// 		Username string `json:"username"`
// 		Password string `json:"password"`
// 		UserUUID string `json:"userUUID"`
// 	}
// )

var (
	address     = flag.String("address", "127.0.0.1:25565", "The server address")
	name        = flag.String("name", "Daze", "The player's name")
	playerID    = flag.String("uuid", "", "The player's UUID")
	mcVersion   = flag.String("version", "1.21.5", "target version to connect client to")
	offline     = flag.Bool("offline", false, "use offline mode")
	accessToken = flag.String("token", "", "AccessToken - only used in offline mode")
	// authFile    = flag.String("auth", "", "json file containing auth credentials")
)

var (
	client        *bot.Client
	player        *basic.Player
	playerList    *playerlist.PlayerList
	chatHandler   *msg.Manager
	worldManager  *world.World
	screenManager *screen.Manager

	blockMgr  mc_versions.BlockMgr
	soundMgr  mc_versions.SoundMgr
	packetMgr protocol_models.PacketMgr

	movementExecutor movement.MovementExecutor

	protocolVersion uint

	// Bot position tracking
	botPosition struct {
		mu          sync.RWMutex
		X, Y, Z     float64
		Yaw, Pitch  float32
		initialized bool
	}

	// Entity tracking for nearby players
	trackedEntities struct {
		mu       sync.RWMutex
		entities map[int32]*TrackedEntity
	}

	// Custom registry storage for registries not in Tnze/go-mc
	customRegistries struct {
		mu         sync.RWMutex
		registries map[string]*CustomRegistry // registry ID -> registry data
	}
)

// CustomRegistry stores a single registry's data (ID -> name mappings)
type CustomRegistry struct {
	ID     string           // Registry ID (e.g., "minecraft:entity_type")
	ByID   map[int32]string // Numeric ID -> name
	ByName map[string]int32 // Name -> numeric ID (reverse lookup)
	Ready  bool             // Whether the registry has been loaded
}

// TrackedEntity represents a tracked player entity
type TrackedEntity struct {
	EntityID   int32
	EntityType int32
	UUID       [16]byte
	X, Y, Z    float64
	Yaw        int8
	Pitch      int8
}

func init() {
	flag.Parse()

}

// getBotPosition returns the current bot position and rotation
func getBotPosition() (x, y, z float64, yaw, pitch float32, initialized bool) {
	botPosition.mu.RLock()
	defer botPosition.mu.RUnlock()
	return botPosition.X, botPosition.Y, botPosition.Z, botPosition.Yaw, botPosition.Pitch, botPosition.initialized
}

// setBotPosition updates the bot position and rotation
func setBotPosition(x, y, z float64, yaw, pitch float32) {
	botPosition.mu.Lock()
	defer botPosition.mu.Unlock()
	botPosition.X = x
	botPosition.Y = y
	botPosition.Z = z
	botPosition.Yaw = yaw
	botPosition.Pitch = pitch
	botPosition.initialized = true
}

func main() {
	// TO DO: Rewrite client/player/etc. to use packetMgr instead of tnze bot (mc-bot-go) (in progress)

	// log.SetOutput(colorable.NewColorableStdout())

	// fmt.Printf("checking for authfile (%s)\n", *authFile)
	// if _, err := os.Stat(*authFile); err == nil || !*offline {
	// 	fmt.Printf("found authfile, loading creds")
	// 	credentials, err := loadCredFile(*authFile)
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	fmt.Printf("Logging in as %s\n", credentials.Username)

	var auth bot.Auth

	if !*offline {
		cid := "88650e7e-efee-4857-b9a9-cf580a00ef43" // MS app ID from msauth mod documentation.  Should really use my own - being lazy right now.

		// ms-auth
		// mauth, err := msauth.GetMCcredentials(credentials.Username, credentials.Password)
		// mauth, err := msauth.GetMCcredentials(".credCacheFile", credentials.UserUUID)
		mauth, err := msauth.GetMCcredentials(".credCacheFile", cid) // use cid from msauth README
		if err != nil {
			log.Print(err)
			panic(err)
		}
		log.Print("Authenticated as ", mauth.Name, " (", mauth.UUID, ")")
		auth = bot.Auth{
			AsTk: mauth.AsTk,
			Name: mauth.Name,
			UUID: mauth.UUID,
		}
	} else {
		fmt.Printf("Offline mode => setting bot.Auth to {name: %s, UUID: %s, AsTk: %s}\n", *name, *playerID, *accessToken)
		auth = bot.Auth{
			Name: *name,
			UUID: *playerID,
			AsTk: *accessToken,
		}
	}
	var err error
	var ok bool

	*mcVersion, protocolVersion, err = rof_utils.CheckServerVersion(*address, 0)
	if err != nil {
		panic(err)
	}

	if _, ok = mc_versions.VersionProtocol[*mcVersion]; !ok {
		panic(fmt.Sprintf("unsupported version %v", *mcVersion))
	}

	packetMgr = mc_versions.GetPacketMgrForVersion(*mcVersion)
	blockMgr = mc_versions.GetBlockMgrForVersion(*mcVersion)
	soundMgr = mc_versions.GetSoundMgrForVersion(*mcVersion)

	// Initialize entity tracking
	trackedEntities.entities = make(map[int32]*TrackedEntity)

	// Initialize custom registry storage
	customRegistries.registries = make(map[string]*CustomRegistry)

	client = bot.NewClient(packetMgr)
	client.Auth = auth

	// Hook into configuration phase to capture registry data
	setupRegistryDataCapture(client)

	// Initialize movement executor with bot position access helpers
	movementExecutor = movement.NewMovementExecutor(
		client,
		packetMgr,
		getBotPosition,
		setBotPosition,
	)

	player = basic.NewPlayer(client, basic.DefaultSettings, basic.EventsListener{
		GameStart:    onGameStart,
		Disconnect:   onDisconnect,
		HealthChange: onHealthChange,
		Death:        onDeath,
		Teleported:   onTeleported,
	}, packetMgr)

	playerList = playerlist.New(client, packetMgr)

	chatHandler = msg.New(client, player, playerList, msg.EventsHandler{
		SystemChat:        onSystemMsg,
		PlayerChatMessage: onPlayerMsg,
		DisguisedChat:     onDisguisedMsg,
	}, packetMgr)

	worldManager = world.NewWorld(client, player, world.EventsListener{
		LoadChunk:   onChunkLoad,
		UnloadChunk: onChunkUnload,
	}, packetMgr)

	screenManager = screen.NewManager(client, screen.EventsListener{
		Open:    nil,
		SetSlot: onScreenSlotChange,
		Close:   nil,
	}, packetMgr)

	// Login

	err = client.JoinServerWithOptions(*address, bot.JoinOptions{ProtocolVersion: protocolVersion})
	if err != nil {
		log.Fatal(err)
	}

	startTime := time.Now()
	os.MkdirAll("./logs", 0760)
	logger := &lumberjack.Logger{
		Filename:   "./logs/" + startTime.Format(time.RFC3339) + "_receiver.log", // Location of the log file
		MaxSize:    10,                                                           // Maximum file size (in MB)
		MaxBackups: 3,                                                            // Maximum number of old files to retain
		MaxAge:     28,                                                           // Maximum number of days to retain old files
		Compress:   true,                                                         // Whether to compress/archive old files
		LocalTime:  true,                                                         // Use local time for timestamps
	}

	client.Events.AddGeneric(logPackets(logger, *mcVersion, protocolVersion))

	// client.Events.AddListener(bot.PacketHandler{
	// 	ID:       packetMgr.GetClientboundPacketID("ClientboundPlayerInfo"),
	// 	Priority: 10,
	// 	F: func(p pk.Packet) error {
	// 		pkt, err := packetMgr.GetClientboundPacketByID(protocol_models.ClientboundPacketID(p.ID))
	// 		if err != nil {
	// 			return err
	// 		}

	// 		err = pkt.Scan(p)
	// 		if err != nil {
	// 			return err
	// 		}

	// 		return nil
	// 	},
	// })

	var soundListener = bot.PacketHandler{
		ID:       packetMgr.GetClientboundPacketID("ClientboundSound"),
		Priority: 0,
		F: func(p pk.Packet) error {
			var (
				SoundID pk.VarInt
				// soundEvent    SoundEvent
				SoundCategory pk.VarInt
				X, Y, Z       pk.Int
				Volume, Pitch pk.Float
				Seed          pk.Long
			)
			if err := p.Scan(&SoundID, &SoundCategory, &X, &Y, &Z, &Volume, &Pitch, &Seed); err != nil {
				return err
			}

			// if err := p.Scan(&soundEvent, &SoundCategory, &X, &Y, &Z, &Volume, &Pitch, &Seed); err != nil {
			// 	return err
			// }

			// fmt.Printf("soundId: %#v\tsoundEvent: %#v\n", SoundID, soundEvent)
			return onSound(int32(SoundID), int32(SoundCategory), float64(X)/8, float64(Y)/8, float64(Z)/8, float32(Volume), float32(Pitch), int32(Seed))
		},
	}

	// var handleSetEntityData = bot.PacketHandler{
	// 	ID:       packetMgr.GetClientboundPacketID("ClientboundSetEntityData"),
	// 	Priority: 0,
	// 	F: func(p pk.Packet) error {
	// 		log.Printf("[received packet] %s (ID: %02X Data: % 02X)\n", packetid.ClientboundPacketID(p.ID).String(), p.ID, p.Data)

	// 		var (
	// 			EntityID pk.VarInt
	// 			// Metadata pk.M
	// 			Metadata EntityMetaData

	// 			// index      pk.Byte
	// 			// entityType pk.VarInt
	// 		)
	// 		p.Scan(&EntityID, &Metadata)
	// 		log.Printf("received SetEntityData: EntityID: %d\tMetadata: %#v\n", EntityID, spew.Sdump(Metadata))
	// 		return nil
	// 	},
	// }

	// Entity tracking packet handlers
	// Note: In modern Minecraft (1.19+), players spawn via ClientboundAddEntity like other entities.
	// We track ALL entities and use playerList to identify which are players.

	var handleSpawnEntity = bot.PacketHandler{
		ID:       packetMgr.GetClientboundPacketID("ClientboundAddEntity"),
		Priority: 0,
		F: func(p pk.Packet) error {
			var (
				EntityID   pk.VarInt
				EntityUUID pk.UUID
				EntityType pk.VarInt
				X, Y, Z    pk.Double
				Pitch      pk.Angle
				Yaw        pk.Angle
				HeadYaw    pk.Angle
				Data       pk.VarInt
				VelX       pk.Short
				VelY       pk.Short
				VelZ       pk.Short
			)
			if err := p.Scan(&EntityID, &EntityUUID, &EntityType, &X, &Y, &Z, &Pitch, &Yaw, &HeadYaw, &Data, &VelX, &VelY, &VelZ); err != nil {
				return nil // Ignore scan errors for entity packets
			}

			// Track all entities (we'll filter for players later using playerList)
			trackedEntities.mu.Lock()
			var uuid [16]byte
			copy(uuid[:], EntityUUID[:])
			trackedEntities.entities[int32(EntityID)] = &TrackedEntity{
				EntityID:   int32(EntityID),
				EntityType: int32(EntityType),
				UUID:       uuid,
				X:          float64(X),
				Y:          float64(Y),
				Z:          float64(Z),
				Yaw:        int8(Yaw),
				Pitch:      int8(Pitch),
			}
			trackedEntities.mu.Unlock()

			// Check if this entity is a player by looking in playerList
			var entityUUID [16]byte
			copy(entityUUID[:], EntityUUID[:])
			isPlayer := false
			for uuid := range playerList.PlayerInfos {
				var playerUUID [16]byte
				copy(playerUUID[:], uuid[:])
				if playerUUID == entityUUID {
					isPlayer = true
					break
				}
			}

			if isPlayer {
				log.Printf("Tracking PLAYER entity %d (UUID: %v) at (%.2f, %.2f, %.2f)", EntityID, EntityUUID, X, Y, Z)
			} else {
				log.Printf("Tracking entity %d (type %d) at (%.2f, %.2f, %.2f)", EntityID, EntityType, X, Y, Z)
			}

			return nil
		},
	}

	var handleEntityPosition = bot.PacketHandler{
		ID:       packetMgr.GetClientboundPacketID("ClientboundMoveEntityPos"),
		Priority: 0,
		F: func(p pk.Packet) error {
			var (
				EntityID   pk.VarInt
				DX, DY, DZ pk.Short
				OnGround   pk.Boolean
			)
			if err := p.Scan(&EntityID, &DX, &DY, &DZ, &OnGround); err != nil {
				return nil
			}

			trackedEntities.mu.Lock()
			if entity, ok := trackedEntities.entities[int32(EntityID)]; ok {
				entity.X += float64(DX) / (128 * 32)
				entity.Y += float64(DY) / (128 * 32)
				entity.Z += float64(DZ) / (128 * 32)
			}
			trackedEntities.mu.Unlock()
			return nil
		},
	}

	var handleEntityPositionRotation = bot.PacketHandler{
		ID:       packetMgr.GetClientboundPacketID("ClientboundMoveEntityPosRot"),
		Priority: 0,
		F: func(p pk.Packet) error {
			var (
				EntityID   pk.VarInt
				DX, DY, DZ pk.Short
				Yaw, Pitch pk.Angle
				OnGround   pk.Boolean
			)
			if err := p.Scan(&EntityID, &DX, &DY, &DZ, &Yaw, &Pitch, &OnGround); err != nil {
				return nil
			}

			trackedEntities.mu.Lock()
			if entity, ok := trackedEntities.entities[int32(EntityID)]; ok {
				entity.X += float64(DX) / (128 * 32)
				entity.Y += float64(DY) / (128 * 32)
				entity.Z += float64(DZ) / (128 * 32)
				entity.Yaw = int8(Yaw)
				entity.Pitch = int8(Pitch)
			}
			trackedEntities.mu.Unlock()
			return nil
		},
	}

	var handleTeleportEntity = bot.PacketHandler{
		ID:       packetMgr.GetClientboundPacketID("ClientboundTeleportEntity"),
		Priority: 0,
		F: func(p pk.Packet) error {
			var (
				EntityID   pk.VarInt
				X, Y, Z    pk.Double
				Yaw, Pitch pk.Angle
				OnGround   pk.Boolean
			)
			if err := p.Scan(&EntityID, &X, &Y, &Z, &Yaw, &Pitch, &OnGround); err != nil {
				return nil
			}

			trackedEntities.mu.Lock()
			if entity, ok := trackedEntities.entities[int32(EntityID)]; ok {
				// Absolute position update
				entity.X = float64(X)
				entity.Y = float64(Y)
				entity.Z = float64(Z)
				entity.Yaw = int8(Yaw)
				entity.Pitch = int8(Pitch)
				log.Printf("Entity %d teleported to (%.2f, %.2f, %.2f)", EntityID, X, Y, Z)
			}
			trackedEntities.mu.Unlock()
			return nil
		},
	}

	var handleRemoveEntities = bot.PacketHandler{
		ID:       packetMgr.GetClientboundPacketID("ClientboundRemoveEntities"),
		Priority: 0,
		F: func(p pk.Packet) error {
			var entityIDs []pk.VarInt
			if err := p.Scan(pk.Array(&entityIDs)); err != nil {
				return nil
			}

			trackedEntities.mu.Lock()
			for _, id := range entityIDs {
				delete(trackedEntities.entities, int32(id))
				log.Printf("Stopped tracking entity %d", id)
			}
			trackedEntities.mu.Unlock()
			return nil
		},
	}

	// Custom ClientboundPosition handler for 1.21.5+ with new packet structure
	// Priority 63 to run before basic.Player's handler (priority 64)
	var handleBotPosition = bot.PacketHandler{
		ID:       packetMgr.GetClientboundPacketID("ClientboundPosition"),
		Priority: 63,
		F: func(p pk.Packet) error {
			var (
				TeleportID pk.VarInt
				X, Y, Z    pk.Double
				DX, DY, DZ pk.Double // Delta fields (new in 1.21.5+)
				Yaw, Pitch pk.Float
				Flags      pk.VarInt // PositionUpdateRelatives as VarInt
			)
			if err := p.Scan(&TeleportID, &X, &Y, &Z, &DX, &DY, &DZ, &Yaw, &Pitch, &Flags); err != nil {
				log.Printf("Failed to scan ClientboundPosition packet: %v", err)
				return nil // Don't block other handlers
			}

			// Update bot position
			botPosition.mu.Lock()

			// The X, Y, Z fields are always absolute positions
			botPosition.X = float64(X)
			botPosition.Y = float64(Y)
			botPosition.Z = float64(Z)

			// Yaw and Pitch handling based on flags
			// Flags bitfield: 0x01=X, 0x02=Y, 0x04=Z, 0x08=Yaw, 0x10=Pitch (for delta/velocity)
			// For rotation, check if relative
			if Flags&0x08 != 0 {
				botPosition.Yaw += float32(Yaw)
			} else {
				botPosition.Yaw = float32(Yaw)
			}

			if Flags&0x10 != 0 {
				botPosition.Pitch += float32(Pitch)
			} else {
				botPosition.Pitch = float32(Pitch)
			}

			botPosition.initialized = true

			log.Printf("Bot position updated via ClientboundPosition (flags=0x%X): X=%.2f, Y=%.2f, Z=%.2f, Yaw=%.2f, Pitch=%.2f (Delta: %.2f, %.2f, %.2f)",
				Flags, botPosition.X, botPosition.Y, botPosition.Z, botPosition.Yaw, botPosition.Pitch, DX, DY, DZ)

			botPosition.mu.Unlock()

			// Accept the teleportation
			return player.AcceptTeleportation(TeleportID)
		},
	}

	client.Events.AddListener(soundListener)
	client.Events.AddListener(handleBotPosition)
	client.Events.AddListener(handleSpawnEntity)
	client.Events.AddListener(handleEntityPosition)
	client.Events.AddListener(handleEntityPositionRotation)
	client.Events.AddListener(handleTeleportEntity)
	client.Events.AddListener(handleRemoveEntities)
	// client.Events.AddListener(handleSetEntityData)

	log.Println("Login success")

	fmt.Println("Starting game loop")
	// JoinGame
	for {
		var err error
		if err = client.HandleGame(); err == nil {
			panic("HandleGame should never return nil")
		}

		if err2 := new(bot.PacketHandlerError); errors.As(err, err2) {
			if err := new(DisconnectErr); errors.As(err2, err) {
				log.Print("Disconnect, reason: ", err.Reason)
				return
			} else {
				// print and ignore the error
				log.Print(err2)
			}
		} else {
			log.Fatal(err)
		}
	}
}

func onDeath() error {
	log.Println("Died and Respawned")
	// If we exclude Respawn(...) then the player won't press the "Respawn" button upon death
	go func() {
		time.Sleep(time.Second * 5)
		err := player.Respawn()
		if err != nil {
			log.Print(err)
		}
	}()
	return nil
}

func onGameStart() error {
	log.Println("Game start")

	// Log registry status
	customRegistries.mu.RLock()
	registryCount := len(customRegistries.registries)
	customRegistries.mu.RUnlock()
	log.Printf("Loaded %d custom registries during configuration", registryCount)

	if err := chatHandler.SendMessage("Hello, world"); err != nil {
		return err
	}
	return nil // if err isn't nil, HandleGame() will return it.
}

// setupRegistryDataCapture hooks into the configuration phase to capture registry data
func setupRegistryDataCapture(c *bot.Client) {
	// Store the original JoinConfiguration function
	// originalJoinConfig := c.JoinConfiguration
	// if originalJoinConfig == nil {
	// 	// Use the default from bot package
	// 	originalJoinConfig = c.JoinConfiguration
	// }

	// // Wrap it to capture registry packets
	// c.JoinConfiguration = func(conn *mcnet.Conn) error {
	// 	// Call the original configuration handler which will process all config packets
	// 	// We'll add a listener to intercept RegistryData packets before they're fully processed
	// 	return originalJoinConfig(conn)
	// }

	// Add a high-priority event listener for RegistryData packets
	// This will run during configuration phase
	client.Events.AddListener(bot.PacketHandler{
		ID:       packetMgr.GetClientboundConfigPacketID("ClientboundConfigRegistryData"),
		Priority: 100, // High priority to capture before default handler
		F:        handleRegistryDataPacket,
	})
}

// handleRegistryDataPacket processes ClientboundConfigRegistryData packets to build registries
func handleRegistryDataPacket(p pk.Packet) error {
	// Create a reader for the packet data
	reader := bytes.NewReader(p.Data)

	// Read registry ID
	var registryID pk.String
	if _, err := registryID.ReadFrom(reader); err != nil {
		log.Printf("Failed to read registry ID: %v", err)
		return nil // Don't block configuration
	}

	log.Printf("Received registry data for: %s", registryID)

	// Read number of entries
	var numEntries pk.VarInt
	if _, err := numEntries.ReadFrom(reader); err != nil {
		log.Printf("Failed to read number of entries for %s: %v", registryID, err)
		return nil
	}

	log.Printf("Registry %s has %d entries", registryID, numEntries)

	// Create a new registry
	registry := &CustomRegistry{
		ID:     string(registryID),
		ByID:   make(map[int32]string),
		ByName: make(map[string]int32),
		Ready:  false,
	}

	// Read each entry
	for i := 0; i < int(numEntries); i++ {
		var entryKey pk.String
		var hasData pk.Boolean

		// Read entry name
		if _, err := entryKey.ReadFrom(reader); err != nil {
			log.Printf("Failed to read entry %d key: %v", i, err)
			break
		}

		// Read hasData flag
		if _, err := hasData.ReadFrom(reader); err != nil {
			log.Printf("Failed to read entry %d hasData: %v", i, err)
			break
		}

		// Store the mapping (index -> name)
		registry.ByID[int32(i)] = string(entryKey)
		registry.ByName[string(entryKey)] = int32(i)

		// Skip NBT data if present (we only need the names)
		if hasData {
			// Read and discard the NBT data
			var nbtData protocol_models.NBTField
			if _, err := nbtData.ReadFrom(reader); err != nil {
				log.Printf("Warning: Failed to skip NBT data for entry %d (%s): %v", i, entryKey, err)
				// Continue anyway
			}
		}
	}

	// Mark registry as ready
	registry.Ready = true

	// Store the registry
	customRegistries.mu.Lock()
	customRegistries.registries[string(registryID)] = registry
	customRegistries.mu.Unlock()

	log.Printf("Successfully loaded registry %s with %d entries", registryID, len(registry.ByID))

	return nil // Return nil to allow configuration to continue
}

// getCustomRegistry retrieves a custom registry by ID (thread-safe)
func getCustomRegistry(registryID string) *CustomRegistry {
	customRegistries.mu.RLock()
	defer customRegistries.mu.RUnlock()
	return customRegistries.registries[registryID]
}

func onSystemMsg(c chat.Message, overlay bool) error {
	log.Printf("System Chat: %#v, Overlay: %v", c, overlay)
	return nil
}

func onPlayerMsg(senderInfo playerlist.PlayerInfo, msg chat.Message, validated bool) error {
	const rofBotFlag = ">>>ROF_bot<<<"
	var prefix string
	if !validated {
		prefix = "[Not Secure] "
	}
	log.Printf("\n%sPlayer: %v\n\n", prefix, msg)

	if senderInfo.ID.String() == client.UUID.String() {
		return nil
	}

	if strings.Contains(msg.String(), rofBotFlag) {
		// find raw message...
		text := ""
		for _, with := range msg.With {
			if strings.HasPrefix(with.Text, ">>>ROF_bot<<<") {
				text = strings.TrimPrefix(with.Text, rofBotFlag)
				break
			}
		}
		if text != "" {
			text = strings.TrimSpace(text)
			chatHandler.SendMessage("Received: " + text)
			handleChatCommand(text)
		}
	} else {
		// echo back the chat to the sender
		err := chatHandler.SendMessage("received " + msg.String())
		if err != nil {
			return err
		}
	}
	return nil
}

func onDisguisedMsg(msg chat.Message) error {
	log.Printf("Disguised: %v", msg)
	return nil
}

func onChunkLoad(pos level.ChunkPos) error {
	log.Println("Loaded chunk:", pos)
	log.Printf("%#v\n", worldManager.Columns[pos])
	spew.Dump(worldManager.Columns[pos])
	return nil
}

func onChunkUnload(pos level.ChunkPos) error {
	log.Println("Unload chunk:", pos)
	log.Printf("%#v\n", worldManager.Columns[pos])
	return nil
}

func onScreenSlotChange(id, index int) error {
	if id == -2 {
		log.Printf("Slot: inventory: %v", screenManager.Inventory.Slots[index])
	} else if id == -1 && index == -1 {
		log.Printf("Slot: cursor: %v", screenManager.Cursor)
	} else {
		container, ok := screenManager.Screens[id]
		if ok {
			// Currently, only inventory container is supported
			switch container := container.(type) {
			case *screen.Inventory:
				slot := container.Slots[index]
				itemName := "nil"
				if slot.ID >= 0 && int(slot.ID) < len(registryid.Item) {
					itemName = registryid.Item[slot.ID]
				}
				log.Printf("Slot: Screen[%d].Slot[%d]: [%v] * %d | NBT: %v", id, index, itemName, slot.Count, slot.NBT)
			}
		}
	}
	return nil
}

func onHealthChange(health float32, foodLevel int32, foodSaturation float32) error {
	log.Printf("Health: %.2f, FoodLevel: %d, FoodSaturation: %.2f", health, foodLevel, foodSaturation)
	return nil
}

func onTeleported(x, y, z float64, yaw, pitch float32, flags byte, teleportID int32) error {
	botPosition.mu.Lock()
	defer botPosition.mu.Unlock()

	// Handle relative vs absolute positioning based on flags
	// Flags: 0x01=X relative, 0x02=Y relative, 0x04=Z relative, 0x08=Yaw relative, 0x10=Pitch relative
	if flags&0x01 != 0 {
		botPosition.X += x
	} else {
		botPosition.X = x
	}

	if flags&0x02 != 0 {
		botPosition.Y += y
	} else {
		botPosition.Y = y
	}

	if flags&0x04 != 0 {
		botPosition.Z += z
	} else {
		botPosition.Z = z
	}

	if flags&0x08 != 0 {
		botPosition.Yaw += yaw
	} else {
		botPosition.Yaw = yaw
	}

	if flags&0x10 != 0 {
		botPosition.Pitch += pitch
	} else {
		botPosition.Pitch = pitch
	}

	botPosition.initialized = true

	log.Printf("Bot position updated (flags=0x%02X): X=%.2f, Y=%.2f, Z=%.2f, Yaw=%.2f, Pitch=%.2f",
		flags, botPosition.X, botPosition.Y, botPosition.Z, botPosition.Yaw, botPosition.Pitch)

	// Accept the teleportation
	return player.AcceptTeleportation(pk.VarInt(teleportID))
}

type DisconnectErr struct {
	Reason chat.Message
}

func (d DisconnectErr) Error() string {
	return "disconnect: " + d.Reason.String()
}

func onDisconnect(reason chat.Message) error {
	// return an error value so that we can stop main loop
	return DisconnectErr{Reason: reason}
}

type SoundEvent struct {
	ID            pk.Identifier
	HasFixedRange pk.Boolean
	FixedRange    pk.Float
}

func (se *SoundEvent) ReadFrom(r io.Reader) (int64, error) {
	var n1, n2, n3 int64
	var err error
	n1, err = se.ID.ReadFrom(r)
	if err != nil {
		return 0, err
	}

	n2, err = se.HasFixedRange.ReadFrom(r)
	if err != nil {
		return 0, err
	}

	if se.HasFixedRange {
		n3, err = se.FixedRange.ReadFrom(r)
		if err != nil {
			return 0, err
		}
	}
	return n1 + n2 + n3, err
}

func logPackets(logFile io.Writer, version string, protocolVersion uint) bot.PacketHandler {
	return bot.PacketHandler{
		Priority: 0,
		F: func(p pk.Packet) error {
			log.Printf("[received packet] %s (ID: 0x%X) [%v] len: %d", packetMgr.ClientboundToString(protocol_models.ClientboundPacketID(p.ID)), p.ID, p.Data[:min(20, len(p.Data))], len(p.Data))
			if logFile != nil {
				pl := protocol_models.PacketLog{
					ID:              p.ID,
					Data:            make([]byte, len(p.Data)),
					Name:            packetMgr.ClientboundToString(protocol_models.ClientboundPacketID(p.ID)),
					Timestamp:       time.Now(),
					Version:         version,
					ProtocolVersion: protocolVersion,
				}
				copy(pl.Data, p.Data)
				err := json.NewEncoder(logFile).Encode(pl)
				if err != nil {
					log.Printf("[ERROR] failed to write to receiver log: %#v", err)
					return err
				}
			}
			return nil
		},
	}
}

type Optional[T pk.FieldDecoder] struct {
	HasData pk.Boolean
	Data    T
}

func (o *Optional[T]) ReadFrom(r io.Reader) (n int64, err error) {
	nn, err := o.HasData.ReadFrom(r)
	if err != nil {
		return nn, err
	}
	n += nn
	if o.HasData {
		nn, err = o.Data.ReadFrom(r)
		if err != nil {
			return nn, err
		}
		n += nn
	}
	return n, err
}

type Optional2Vals[T1, T2 pk.FieldDecoder] struct {
	HasData pk.Boolean
	Data1   T1
	Data2   T2
}

func (o Optional2Vals[T1, T2]) ReadFrom(r io.Reader) (n int64, err error) {
	nn, err := o.HasData.ReadFrom(r)
	if err != nil {
		return nn, err
	}
	n += nn
	if o.HasData {
		nn, err = o.Data1.ReadFrom(r)
		if err != nil {
			return nn, err
		}
		n += nn

		nn, err = o.Data2.ReadFrom(r)
		if err != nil {
			return nn, err
		}
		n += nn
	}

	return n, err
}

type Particle struct {
	ID   pk.VarInt
	Data pk.FieldDecoder
}

func (p *Particle) ReadFrom(r io.Reader) (n int64, err error) {
	var particleID pk.VarInt
	nn, err := particleID.ReadFrom(r)
	if err != nil {
		return nn, err
	}
	n += nn
	switch particleID {
	case 1: // minecraft:block
		var val pk.VarInt
		p.Data = &val
	case 2: // minecraft:block_marker
		var val pk.VarInt
		p.Data = &val
	case 13: // minecraft:dust
		var val DustParticle
		p.Data = &val
	case 14: // minecraft:dust_color_transition
		var val DustColorTransitionParticle
		p.Data = &val
	case 20: // minecraft:entity_effect
		var val pk.Int
		p.Data = &val
	case 28: // minecraft:falling_dust
		var val pk.VarInt
		p.Data = &val
	case 35: // minecraft:sculk_charge
		var val pk.Float
		p.Data = &val
	case 44: // minecraft:item
		var val SlotParticle
		p.Data = &val
	case 45: // minecraft:vibration
		var val VibrationParticle
		p.Data = &val
	case 99: // minecraft:shriek
		var val pk.VarInt
		p.Data = &val
	case 105: // minecraft:dust_pillar
		var val pk.VarInt
		p.Data = &val

	}

	if p.Data != nil {
		nn, err = p.Data.ReadFrom(r)
		if err != nil {
			return nn, err
		}
	}

	return n, err
}

type DustParticle struct {
	R     pk.Float
	G     pk.Float
	B     pk.Float
	Scale pk.Float
}

func (p *DustParticle) ReadFrom(r io.Reader) (n int64, err error) {
	for _, particle := range []pk.Float{p.R, p.G, p.B, p.Scale} {
		nn, err := particle.ReadFrom(r)
		if err != nil {
			return nn, err
		}
		n += nn
	}
	return n, err
}

type DustColorTransitionParticle struct {
	ToR   pk.Float
	ToG   pk.Float
	ToB   pk.Float
	FromR pk.Float
	FromG pk.Float
	FromB pk.Float
	Scale pk.Float
}

func (p *DustColorTransitionParticle) ReadFrom(r io.Reader) (n int64, err error) {
	for _, particle := range []pk.Float{p.ToR, p.ToG, p.ToB, p.FromR, p.FromG, p.FromB, p.Scale} {
		nn, err := particle.ReadFrom(r)
		if err != nil {
			return nn, err
		}
		n += nn
	}
	return n, err
}

type VibrationParticle struct {
	PositionSourceType pk.VarInt   // The type of the vibration source (0 for `minecraft:block`, 1 for `minecraft:entity`)
	BlockPosition      pk.Position // The position of the block the vibration originated from. Only present if Position Type is minecraft:block.
	EntityID           pk.VarInt   // The ID of the entity the vibration originated from. Only present if Position Type is minecraft:entity.
	EntityEyeHeight    pk.Float    // The height of the entity's eye relative to the entity. Only present if Position Type is minecraft:entity.
	Ticks              pk.VarInt   // The amount of ticks it takes for the vibration to travel from its source to its destination.
}

func (vp *VibrationParticle) ReadFrom(r io.Reader) (n int64, err error) {
	nn, err := vp.PositionSourceType.ReadFrom(r)
	if err != nil {
		return nn, err
	}

	switch vp.PositionSourceType {
	case 0: // minecraft:block
		nn, err = vp.BlockPosition.ReadFrom(r)
		if err != nil {
			return nn, err
		}
	case 1: // minecraft:entity
		nn, err = vp.EntityID.ReadFrom(r)
		if err != nil {
			return nn, err
		}

		nn, err = vp.EntityEyeHeight.ReadFrom(r)
		if err != nil {
			return nn, err
		}
	default:
		return 0, fmt.Errorf("unknown position source type: %v", vp.PositionSourceType)
	}

	nn, err = vp.Ticks.ReadFrom(r)
	if err != nil {
		return nn, err
	}

	return n, err
}

type SlotParticle struct {
	ItemCount   pk.VarInt
	ItemID      pk.VarInt
	AddCount    pk.VarInt
	RemoveCount pk.VarInt
	Add         []pk.FieldDecoder
	Remove      []pk.VarInt
}

func (sp *SlotParticle) ReadFrom(r io.Reader) (n int64, err error) {
	return n, err
}

type EntityMetaData struct {
	Properties []Property
}

func (emd *EntityMetaData) ReadFrom(r io.Reader) (n int64, err error) {
	var property Property
	const EOF = 0xFF

	for {
		nn, err := property.Index.ReadFrom(r)
		if err != nil {
			return nn, err
		}
		n += nn
		fmt.Printf("\tindex: %d\n", property.Index)

		if property.Index == EOF {
			fmt.Printf("index == EOF\n")
			return n, nil
		}

		nn, err = property.Type.ReadFrom(r)
		if err != nil {
			return n, err
		}
		n += nn
		fmt.Printf("\tentType: %d\n", property.Type)

		// TO DO: READ VALUE BASED ON entType
		switch property.Type {
		case 0: // Byte
			var val pk.Byte
			property.Value = &val
		case 1: //VarInt
			var val pk.VarInt
			property.Value = &val
		case 2: // VarLong
			var val pk.VarLong
			property.Value = &val
		case 3:
			var val pk.Float
			property.Value = &val
		case 4:
			var val pk.String
			property.Value = &val
		case 5:
			var val pk.ByteArray
			property.Value = &val
		case 6:
			var val pk.VarInt
			property.Value = &val
		case 7: //Slot
			var val pk.VarInt
			property.Value = &val
		case 8:
			var val pk.Boolean
			property.Value = &val
		case 9: // Rotations
			var val pk.VarInt
			property.Value = &val
		case 10: // Position
			var val pk.Position
			property.Value = &val
		case 11: // OptionalPosition
			var val Optional[*pk.Position]
			val.Data = &pk.Position{}
			property.Value = &val
		case 12:
			var val pk.VarInt
			property.Value = &val
		case 13:
			var val pk.VarInt
			property.Value = &val
		case 14:
			var val pk.VarInt
			property.Value = &val
		case 15:
			var val pk.VarInt
			property.Value = &val
		case 16:
			var val pk.VarInt
			property.Value = &val
		case 17: // Particle
			var val pk.VarInt
			property.Value = &val
		case 18: // Particles
			var val pk.VarInt

			property.Value = &val
		case 19:
			var val pk.VarInt
			property.Value = &val
		case 20:
			var val pk.VarInt
			property.Value = &val
		case 21:
			var val pk.VarInt
			property.Value = &val
		case 22:
			var val pk.VarInt
			property.Value = &val
		case 23:
			var val pk.VarInt
			property.Value = &val
		case 24:
			var val pk.VarInt
			property.Value = &val
		case 25: // Optional global position (optional dimension ID, optional Position)
			var val Optional2Vals[*pk.Identifier, *pk.Position]
			property.Value = &val
		case 26: // ID or painting variant
			var val pk.VarInt
			property.Value = &val

		case 27:
			var val pk.Identifier
			property.Value = &val
		case 28:
			var val pk.ByteArray //pk.Vector3
			property.Value = &val
		case 29:
			var val pk.ByteArray //pk.Quaternion
			property.Value = &val

		}

		if property.Value != nil {
			nn, err = property.Value.ReadFrom(r)
			if err != nil {
				return n, err
			}
			n += nn
			fmt.Printf("\tproperty.Value: %#v\n", property.Value)
			emd.Properties = append(emd.Properties, property)
		} else {
			return n, fmt.Errorf("[EnitityMetaData:ReadFrom] invalid type found")
		}
	}

}

type Property struct {
	Index pk.UnsignedByte
	Type  pk.VarInt
	Value pk.FieldDecoder
}

func ReadFrom[T pk.FieldDecoder](val T, r io.Reader) (T, int64, error) {
	n, err := val.ReadFrom(r)

	return val, n, err
}

func onSound(id, category int32, x, y, z float64, volume, pitch float32, seed int32) error {
	// log.Printf("[1.20.3 soundid]: %d => %s => %s", id, soundid_1_20_3.SoundNames[soundid_1_20_3.SoundID(id)], soundid_1_20_3.SoundSubtitles[soundid_1_20_3.SoundID(id)])
	log.Printf("[%s soundid]: %d => %s => %s", packetMgr.Name(), id, soundMgr.GetSoundNameByID(protocol_models.SoundID(id)), soundMgr.GetSubtitleKeyByID(protocol_models.SoundID(id)))
	// log.Printf("[1.21.3 soundid]: %d => %s => %s", id, soundid_1_21_3.SoundNames[soundid_1_21_3.SoundID(id)], soundid_1_21_3.SoundSubtitles[soundid_1_21_3.SoundID(id)])
	return nil
}

// func loadCredFile(fileName string) (*authCreds, error) {
// 	file, err := os.Open(fileName)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to open %s: %v", fileName, err)
// 	}

// 	contents, err := io.ReadAll(file)
// 	if err != nil {
// 		return nil, err
// 	}

// 	data := authCreds{}

// 	err = json.Unmarshal(contents, &data)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal auth credentials: %v", err)
// 	}

// 	return &data, nil
// }

var fireBowMU sync.Mutex

var (
	trackingActive       bool
	trackingStop         chan struct{}
	trackingStopOnce     sync.Once
	trackingMU           sync.Mutex
	lastNoPlayersMessage time.Time
)

func handleChatCommand(cmd string) {
	cmd = strings.TrimSpace(cmd)
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	command := parts[0]
	args := parts[1:]

	switch command {
	case "fireBow":
		go fireBow()
	case "startTracking":
		go startTracking()
	case "stopTracking":
		stopTracking()
	case "testMove":
		go testMove()
	case "moveTo":
		if len(args) < 3 {
			chatHandler.SendMessage("Usage: moveTo <x> <y> <z>")
			return
		}
		go moveToCommand(args[0], args[1], args[2])
	case "moveForward":
		if len(args) < 1 {
			chatHandler.SendMessage("Usage: moveForward <distance>")
			return
		}
		go moveForwardCommand(args[0])
	case "moveUp":
		if len(args) < 1 {
			chatHandler.SendMessage("Usage: moveUp <distance>")
			return
		}
		go moveUpCommand(args[0])
	default:
		fmt.Printf("unknown command: [%s]", cmd)
	}
}

// testMove performs a simple test movement (move 1 block forward using incremental steps)
func testMove() {
	x, y, z, _, _, initialized := getBotPosition()
	if !initialized {
		chatHandler.SendMessage("Bot position not initialized")
		return
	}

	chatHandler.SendMessage(fmt.Sprintf("Current position: %.2f, %.2f, %.2f", x, y, z))

	// Move 1 block in the +X direction incrementally
	targetX := x + 1.0
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond

	stepCount := int(math.Ceil(1.0 / stepSize))

	for i := 0; i < stepCount; i++ {
		progress := float64(i+1) / float64(stepCount)
		if progress > 1.0 {
			progress = 1.0
		}

		nextX := x + 1.0*progress

		err := movementExecutor.SendPosition(nextX, y, z, true)
		if err != nil {
			chatHandler.SendMessage(fmt.Sprintf("Movement failed: %v", err))
			fmt.Printf("testMove error: %v\n", err)
			return
		}

		time.Sleep(stepDelay)
	}

	chatHandler.SendMessage(fmt.Sprintf("Moved to: %.2f, %.2f, %.2f", targetX, y, z))
	fmt.Printf("testMove: Successfully moved from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f)\n", x, y, z, targetX, y, z)
}

// moveToCommand moves the bot to specific coordinates using incremental movement
func moveToCommand(xStr, yStr, zStr string) {
	var targetX, targetY, targetZ float64
	var err error

	if targetX, err = parseFloat(xStr); err != nil {
		chatHandler.SendMessage(fmt.Sprintf("Invalid X coordinate: %s", xStr))
		return
	}
	if targetY, err = parseFloat(yStr); err != nil {
		chatHandler.SendMessage(fmt.Sprintf("Invalid Y coordinate: %s", yStr))
		return
	}
	if targetZ, err = parseFloat(zStr); err != nil {
		chatHandler.SendMessage(fmt.Sprintf("Invalid Z coordinate: %s", zStr))
		return
	}

	x, y, z, _, _, initialized := getBotPosition()
	if !initialized {
		chatHandler.SendMessage("Bot position not initialized")
		return
	}

	// Calculate distance
	dx := targetX - x
	dy := targetY - y
	dz := targetZ - z
	totalDistance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	chatHandler.SendMessage(fmt.Sprintf("Moving from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) [%.2f blocks]", x, y, z, targetX, targetY, targetZ, totalDistance))

	// First look at the target
	err = movementExecutor.LookAt(targetX, targetY, targetZ, true)
	if err != nil {
		chatHandler.SendMessage(fmt.Sprintf("Look failed: %v", err))
		return
	}

	// Move incrementally to respect server movement speed limits
	// Walking speed is ~4.3 blocks/second, at 20 TPS that's ~0.215 blocks/tick
	// We'll use 0.2 blocks per step to be safe
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond // 20 TPS = 50ms per tick

	stepCount := int(math.Ceil(totalDistance / stepSize))
	if stepCount == 0 {
		chatHandler.SendMessage("Already at target position")
		return
	}

	fmt.Printf("moveToCommand: Moving in %d steps of %.2f blocks\n", stepCount, stepSize)

	for i := 0; i < stepCount; i++ {
		// Calculate next position
		progress := float64(i+1) / float64(stepCount)
		if progress > 1.0 {
			progress = 1.0
		}

		nextX := x + dx*progress
		nextY := y + dy*progress
		nextZ := z + dz*progress

		// Send position update
		err = movementExecutor.SendPosition(nextX, nextY, nextZ, true)
		if err != nil {
			chatHandler.SendMessage(fmt.Sprintf("Movement failed at step %d: %v", i+1, err))
			fmt.Printf("moveToCommand error at step %d: %v\n", i+1, err)
			return
		}

		// Wait for next tick
		time.Sleep(stepDelay)
	}

	chatHandler.SendMessage(fmt.Sprintf("Arrived at (%.2f, %.2f, %.2f)", targetX, targetY, targetZ))
	fmt.Printf("moveToCommand: Successfully moved to (%.2f, %.2f, %.2f)\n", targetX, targetY, targetZ)
}

// moveForwardCommand moves the bot forward by a given distance using incremental movement
func moveForwardCommand(distStr string) {
	distance, err := parseFloat(distStr)
	if err != nil {
		chatHandler.SendMessage(fmt.Sprintf("Invalid distance: %s", distStr))
		return
	}

	x, y, z, yaw, _, initialized := getBotPosition()
	if !initialized {
		chatHandler.SendMessage("Bot position not initialized")
		return
	}

	// Calculate forward direction based on yaw
	// Yaw 0 is south (+Z), 90 is west (-X), 180 is north (-Z), 270 is east (+X)
	yawRad := float64(yaw) * math.Pi / 180
	dx := -math.Sin(yawRad) * distance
	dz := math.Cos(yawRad) * distance

	targetX := x + dx
	targetZ := z + dz

	chatHandler.SendMessage(fmt.Sprintf("Moving forward %.2f blocks", distance))

	// Move incrementally
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond

	totalDistance := math.Abs(distance)
	stepCount := int(math.Ceil(totalDistance / stepSize))

	for i := 0; i < stepCount; i++ {
		progress := float64(i+1) / float64(stepCount)
		if progress > 1.0 {
			progress = 1.0
		}

		nextX := x + dx*progress
		nextZ := z + dz*progress

		err = movementExecutor.SendPosition(nextX, y, nextZ, true)
		if err != nil {
			chatHandler.SendMessage(fmt.Sprintf("Movement failed: %v", err))
			fmt.Printf("moveForwardCommand error: %v\n", err)
			return
		}

		time.Sleep(stepDelay)
	}

	chatHandler.SendMessage(fmt.Sprintf("Moved to (%.2f, %.2f, %.2f)", targetX, y, targetZ))
	fmt.Printf("moveForwardCommand: Moved forward %.2f blocks to (%.2f, %.2f, %.2f)\n", distance, targetX, y, targetZ)
}

// moveUpCommand moves the bot up/down by a given distance using incremental movement
func moveUpCommand(distStr string) {
	distance, err := parseFloat(distStr)
	if err != nil {
		chatHandler.SendMessage(fmt.Sprintf("Invalid distance: %s", distStr))
		return
	}

	x, y, z, _, _, initialized := getBotPosition()
	if !initialized {
		chatHandler.SendMessage("Bot position not initialized")
		return
	}

	targetY := y + distance

	chatHandler.SendMessage(fmt.Sprintf("Moving %.2f blocks vertically", distance))

	// Move incrementally
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond

	totalDistance := math.Abs(distance)
	stepCount := int(math.Ceil(totalDistance / stepSize))

	for i := 0; i < stepCount; i++ {
		progress := float64(i+1) / float64(stepCount)
		if progress > 1.0 {
			progress = 1.0
		}

		nextY := y + distance*progress

		// OnGround = false when moving up, true when on ground
		onGround := nextY <= y
		err = movementExecutor.SendPosition(x, nextY, z, onGround)
		if err != nil {
			chatHandler.SendMessage(fmt.Sprintf("Movement failed: %v", err))
			fmt.Printf("moveUpCommand error: %v\n", err)
			return
		}

		time.Sleep(stepDelay)
	}

	chatHandler.SendMessage(fmt.Sprintf("Moved to (%.2f, %.2f, %.2f)", x, targetY, z))
	fmt.Printf("moveUpCommand: Moved vertically %.2f blocks to (%.2f, %.2f, %.2f)\n", distance, x, targetY, z)
}

// parseFloat is a helper to parse float64 from string with better error handling
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func fireBow() error {
	fireBowMU.Lock()
	defer fireBowMU.Unlock()

	if err := DoUseItem(pk.VarInt(UseItem_MainHand), 1, 0, 0); err != nil {
		return err
	}

	for range 10 {
		if err := DoPlayerAction(PlayerAction_BeginDigging, pk.Position{X: 0, Y: 0, Z: 0}); err != nil {
			// if err := DoPlayerAction(PlayerAction_ShootArrow, pk.Position{X: 0, Y: 0, Z: 0}); err != nil {
			return err
		}

		time.Sleep(time.Second)
	}

	if err := DoPlayerAction(PlayerAction_ShootArrow, pk.Position{X: 0, Y: 0, Z: 0}); err != nil {
		return err
	}

	return nil
}

type UseItem int32

const (
	UseItem_MainHand UseItem = 0
	UseItem_OffHand  UseItem = 1
)

func DoUseItem(hand pk.VarInt, sequence pk.VarInt, yaw pk.Float, pitch pk.Float) error {
	pkt, err := packetMgr.GetServerboundPacketByID(packetMgr.GetServerboundPacketID("ServerboundUseItem"))
	if err != nil {
		return err
	}

	fields := pkt.GetFields()
	fields["Hand"] = pk.VarInt(hand)
	fields["Sequence"] = sequence
	fields["Yaw"] = yaw
	fields["Pitch"] = pitch

	pkt.SetFields(fields)

	return client.Conn.WritePacket(pkt.Marshal())

	// return client.Conn.WritePacket(pk.Marshal(
	// 	 packetid.ServerboundUseItem,
	// 	pk.VarInt(hand),
	// 	sequence,
	// 	yaw,
	// 	pitch,
	// ))
}

type PlayerAction int32

const (
	PlayerAction_BeginDigging  PlayerAction = 0
	PlayerAction_CancelDigging PlayerAction = 1
	PlayerAction_FinishDigging PlayerAction = 2
	PlayerAction_DropItemStack PlayerAction = 3
	PlayerAction_DropItem      PlayerAction = 4
	PlayerAction_ShootArrow    PlayerAction = 5
	PlayerAction_FinishEating  PlayerAction = 5
	PlayerAction_SwapHand      PlayerAction = 6
)

func DoPlayerAction(action PlayerAction, pos pk.Position) error {
	return client.Conn.WritePacket(pk.Marshal(
		// packetid.ServerboundPlayerAction,
		packetMgr.GetServerboundPacketID("ServerboundPlayerAction"),
		pk.VarInt(action),
		pos,
		pk.Byte(0),
		pk.VarInt(0),
	))
}

// calculateLookAngles calculates the yaw and pitch needed to look from (fromX, fromY, fromZ) to (toX, toY, toZ)
func calculateLookAngles(fromX, fromY, fromZ, toX, toY, toZ float64) (yaw, pitch float32) {
	dx := toX - fromX
	dy := toY - fromY
	dz := toZ - fromZ

	// Calculate horizontal distance
	horizontalDist := math.Sqrt(dx*dx + dz*dz)

	// Calculate yaw (rotation around Y axis)
	// Yaw 0 is south (+Z), 90 is west (-X), 180 is north (-Z), 270 is east (+X)
	yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Calculate pitch (rotation around X axis)
	// Pitch -90 is straight up, 0 is level, 90 is straight down
	pitch = float32(-math.Atan2(dy, horizontalDist) * 180 / math.Pi)

	return yaw, pitch
}

// DoLookAt sends a packet to rotate the bot to look at the given position
func DoLookAt(targetX, targetY, targetZ float64) error {
	botPosition.mu.RLock()
	if !botPosition.initialized {
		botPosition.mu.RUnlock()
		return fmt.Errorf("bot position not initialized")
	}
	botX, botY, botZ := botPosition.X, botPosition.Y, botPosition.Z
	botPosition.mu.RUnlock()

	// Calculate look angles from bot's eyes to target's eyes
	// Standard player eye height is 1.62 blocks above feet
	yaw, pitch := calculateLookAngles(botX, botY+1.62, botZ, targetX, targetY+1.62, targetZ)

	// Update our tracked rotation
	botPosition.mu.Lock()
	botPosition.Yaw = yaw
	botPosition.Pitch = pitch
	botPosition.mu.Unlock()

	// Send rotation packet to server
	return client.Conn.WritePacket(pk.Marshal(
		packetMgr.GetServerboundPacketID("ServerboundMovePlayerRot"),
		pk.Float(yaw),
		pk.Float(pitch),
		pk.Boolean(true), // OnGround
	))
}

// NearestPlayerInfo contains information about the nearest player
type NearestPlayerInfo struct {
	EntityID int32
	UUID     [16]byte
	Distance float64
	X, Y, Z  float64
}

// findNearestPlayer calculates the nearest player to the bot
func findNearestPlayer() *NearestPlayerInfo {
	botPosition.mu.RLock()
	if !botPosition.initialized {
		botPosition.mu.RUnlock()
		return nil
	}
	botX, botY, botZ := botPosition.X, botPosition.Y, botPosition.Z
	botPosition.mu.RUnlock()

	trackedEntities.mu.RLock()
	defer trackedEntities.mu.RUnlock()

	if len(trackedEntities.entities) == 0 {
		return nil
	}

	var nearest *NearestPlayerInfo
	minDistance := float64(0)

	for _, entity := range trackedEntities.entities {
		// Check if this entity is a player by looking in playerList
		isPlayer := false
		for uuid := range playerList.PlayerInfos {
			var playerUUID [16]byte
			copy(playerUUID[:], uuid[:])
			if playerUUID == entity.UUID {
				isPlayer = true
				break
			}
		}

		// Only consider player entities
		if !isPlayer {
			continue
		}

		dx := entity.X - botX
		dy := entity.Y - botY
		dz := entity.Z - botZ
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if nearest == nil || distance < minDistance {
			nearest = &NearestPlayerInfo{
				EntityID: entity.EntityID,
				UUID:     entity.UUID,
				Distance: distance,
				X:        entity.X,
				Y:        entity.Y,
				Z:        entity.Z,
			}
			minDistance = distance
		}
	}

	return nearest
}

// formatEntityStats formats entity statistics into a readable string
func formatEntityStats(stats map[string]int) string {
	if len(stats) == 0 {
		return "No entities tracked"
	}

	// Calculate total
	total := 0
	for _, count := range stats {
		total += count
	}

	// Build string with sorted types for consistent output
	var parts []string
	for typeName, count := range stats {
		parts = append(parts, fmt.Sprintf("%s: %d", typeName, count))
	}

	return fmt.Sprintf("Entities (Total: %d) - %s", total, strings.Join(parts, ", "))
}

// getEntityStats returns counts of entities broken down by type
func getEntityStats() map[string]int {
	trackedEntities.mu.RLock()
	defer trackedEntities.mu.RUnlock()

	typeCounts := make(map[string]int)

	// Get entity type registry
	entityTypeRegistry := getCustomRegistry("minecraft:entity_type")

	for _, entity := range trackedEntities.entities {
		// Check if this entity is a player by looking in playerList
		isPlayer := false
		for uuid := range playerList.PlayerInfos {
			var playerUUID [16]byte
			copy(playerUUID[:], uuid[:])
			if playerUUID == entity.UUID {
				isPlayer = true
				break
			}
		}

		var typeName string
		if isPlayer {
			typeName = "Player"
		} else if entityTypeRegistry != nil && entityTypeRegistry.Ready {
			// Look up entity type name from registry if available
			if name, ok := entityTypeRegistry.ByID[entity.EntityType]; ok {
				// Strip "minecraft:" prefix if present
				if strings.HasPrefix(name, "minecraft:") {
					typeName = strings.TrimPrefix(name, "minecraft:")
				} else {
					typeName = name
				}
			} else {
				// Use numeric type ID - registry doesn't have this type yet
				typeName = fmt.Sprintf("type_%d", entity.EntityType)
			}
		} else {
			// Registry not ready yet, use numeric IDs
			typeName = fmt.Sprintf("type_%d", entity.EntityType)
		}

		typeCounts[typeName]++
	}

	return typeCounts
}

// startTracking starts continuous tracking of the nearest player
func startTracking() {
	trackingMU.Lock()
	if trackingActive {
		trackingMU.Unlock()
		chatHandler.SendMessage("Tracking is already active!")
		return
	}
	trackingActive = true
	trackingStop = make(chan struct{})
	trackingStopOnce = sync.Once{}
	lastNoPlayersMessage = time.Time{} // Reset to allow immediate "no players" message
	trackingMU.Unlock()

	chatHandler.SendMessage("Started tracking nearest player...")
	log.Println("Started tracking nearest player")

	ticker := time.NewTicker(time.Second / 20)
	defer ticker.Stop()

	statsTicker := time.NewTicker(15 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case <-trackingStop:
			log.Println("Tracking stopped")
			return
		case <-statsTicker.C:
			// Periodic entity statistics report
			stats := getEntityStats()
			statsMsg := formatEntityStats(stats)
			chatHandler.SendMessage(statsMsg)
			log.Println(statsMsg)
		case <-ticker.C:
			nearest := findNearestPlayer()
			if nearest == nil {
				// Only send "no players" message every 30 seconds to reduce chat spam
				now := time.Now()
				if now.Sub(lastNoPlayersMessage) >= 30*time.Second {
					stats := getEntityStats()
					msg := "No players nearby. " + formatEntityStats(stats)
					chatHandler.SendMessage(msg)
					lastNoPlayersMessage = now
				}
				// Still log it every time for debugging
				stats := getEntityStats()
				log.Printf("No players nearby. %s", formatEntityStats(stats))
			} else {
				// Try to find player name from playerList
				playerName := "Unknown"
				for uuid, info := range playerList.PlayerInfos {
					var infoUUID [16]byte
					copy(infoUUID[:], uuid[:])
					if infoUUID == nearest.UUID {
						playerName = info.GameProfile.Name
						break
					}
				}

				// Make the bot look at the nearest player
				if err := DoLookAt(nearest.X, nearest.Y, nearest.Z); err != nil {
					log.Printf("Failed to look at player: %v", err)
				}
				botPosition.mu.RLock()
				botX, botY, botZ := botPosition.X, botPosition.Y, botPosition.Z
				botPosition.mu.RUnlock()

				msg := fmt.Sprintf("My Pos: (%.1f, %.1f, %.1f) | Nearest: %s | Distance: %.2f blocks | Pos: (%.1f, %.1f, %.1f)",
					botX, botY, botZ, playerName, nearest.Distance, nearest.X, nearest.Y, nearest.Z)
				chatHandler.SendMessage(msg)
				log.Println(msg)
			}
		}
	}
}

// stopTracking stops the continuous tracking
func stopTracking() {
	trackingMU.Lock()
	defer trackingMU.Unlock()

	if !trackingActive {
		chatHandler.SendMessage("Tracking is not active")
		return
	}

	trackingActive = false
	trackingStopOnce.Do(func() {
		close(trackingStop)
	})

	chatHandler.SendMessage("Stopped tracking")
	log.Println("Stopped tracking nearest player")
}
