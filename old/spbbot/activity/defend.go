package activity

import (
	"log"
	"strings"
	"time"

	"github.com/beefsack/go-astar"
	"github.com/spbinns/mc-agent/spbbot/types"

	go_mc_bot "github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/path"
	world_entity "github.com/Tnze/go-mc/bot/world/entity"
	data_entity "github.com/Tnze/go-mc/data/entity"
)

type defend struct {
	client          *go_mc_bot.Client
	inventoryAccess types.InventoryAccess

	running             bool
	callbacksRegistered bool

	currentPath []astar.Pather
	currentStep path.Tile
}

// NewDefendActivity -
func NewDefendActivity(client *go_mc_bot.Client, inventoryAccess types.InventoryAccess) types.Activity {
	return &defend{
		client:          client,
		inventoryAccess: inventoryAccess,
	}
}

func (g *defend) RegisterCallbacks() map[string]func(params ...interface{}) error {
	// if g.callbacksRegistered == false {
	// 	g.callbacksRegistered = true
	// log.Printf("%s Registering callbacks\n", g.GetName())
	return map[string]func(params ...interface{}) error{
		"SoundPlay":          g.onSound,
		"PrePhysicsCallback": g.prePhysicsCallback,
	}
	// }
	// return nil
}

func (g *defend) GetName() string {
	return "Defending"
}

func (g *defend) Start(keywords []string, command string) error {
	g.running = true
	return nil
}

func (g *defend) Stop() error {
	g.running = false
	return nil
}

func (g *defend) IsRunning() bool {
	return g.running
}

func (g *defend) getHostileMobs() []*world_entity.Entity {
	foundHostiles := []*world_entity.Entity{}
	// log.Println(g.client.Wd.Entities)
	for _, e := range g.client.Wd.Entities {
		// log.Printf("%d: %#v\t%#v\n", i, e, e.Base)
		if ent, ok := data_entity.ByID[data_entity.ID(e.Base.ID)]; ok {
			// log.Printf("Looking at mob type %s (%s): %s", ent.DisplayName, ent.Name, ent.Category.String())
			if ent.Category == data_entity.HostileMob {
				if dist(path.V3{X: int(g.client.Player.Pos.X), Y: int(g.client.Player.Pos.Y), Z: int(g.client.Player.Pos.Z)}, path.V3{X: int(e.X), Y: int(e.Y), Z: int(e.Z)}) < 5 {
					foundHostiles = append(foundHostiles, e)
				}
			}
		} else {
			log.Printf("Entity ID not found: %v\n", data_entity.ID(e.ID))
		}
	}
	return foundHostiles
}

func (g *defend) activateSword() {
	slotID := g.client.Player.HeldItem // the slotID for the currently held item (0-8) + 36

	slot := g.inventoryAccess.GetInventoryItem(slotID + 36)

	if !strings.Contains(strings.ToLower(slot.String()), "sword") {
		for i := 0; i < 9; i++ {
			tmpSlot := g.inventoryAccess.GetInventoryItem(i + 36)
			if strings.Contains(strings.ToLower(tmpSlot.String()), "sword") {
				g.client.SelectItem(i)
			}
		}
		newSlotID := g.client.Player.HeldItem
		newSlot := g.inventoryAccess.GetInventoryItem(newSlotID + 36)
		log.Printf("slot[%d] = %#v => slot[%d] = %#v", slotID+36, slot, newSlotID+36, newSlot)
	}
}

func (g *defend) activateBow() {
	slotID := g.client.Player.HeldItem // the slotID for the currently held item (0-8) + 36

	slot := g.inventoryAccess.GetInventoryItem(slotID + 36)

	if !strings.Contains(strings.ToLower(slot.String()), "bow") {
		for i := 0; i < 9; i++ {
			tmpSlot := g.inventoryAccess.GetInventoryItem(i + 36)
			if strings.Contains(strings.ToLower(tmpSlot.String()), "bow") {
				g.client.SelectItem(i)
			}
		}
		newSlotID := g.client.Player.HeldItem
		newSlot := g.inventoryAccess.GetInventoryItem(newSlotID + 36)
		log.Printf("slot[%d] = %#v => slot[%d] = %#v", slotID+36, slot, newSlotID+36, newSlot)
	}
}

func (g *defend) prePhysicsCallback(params ...interface{}) error {
	entityList := g.getHostileMobs()

	var entityToAttack *world_entity.Entity
	// targetX := float64(0)
	// targetY := float64(0)
	// targetZ := float64(0)

	entitiesByDistance := map[float64][]*world_entity.Entity{}

	for _, entity := range entityList {
		distToEntity := dist(g.currentStep.Pos, path.V3{X: int(entity.X), Y: int(entity.Y), Z: int(entity.Z)})
		if _, ok := entitiesByDistance[distToEntity]; ok {
			entitiesByDistance[distToEntity] = append(entitiesByDistance[distToEntity], entity)
		} else {
			entitiesByDistance[distToEntity] = []*world_entity.Entity{entity}
		}

		// if math.Floor(entity.X) == math.Floor(x) && math.Floor(entity.Y) == math.Floor(y) && math.Floor(entity.Z) == math.Floor(z) {
		// entityID = entity.ID
		// targetX = entity.X
		// targetY = entity.Y
		// targetZ = entity.Z
		// 	break
		// }
	}

	//TO DO: ONLY TARGET ENTITIES WE CAN PATH TO - WE ARE CURRENTLY TRYING TO ATTACK ENTITIES THAT ARE UNSEEN!!! (I THINK)

	minDist := float64(100000)
	for theDist, entity := range entitiesByDistance {
		pathFound := g.calculatePath(entity[0]) // THIS IS AN ATTEMPT TO SOLVE THE TARGETING ENTITIES WE CAN PATH TO, BUT IT DIDN'T WORK!
		if pathFound && theDist < minDist {
			minDist = theDist
			entityToAttack = entity[0]
			// targetX = entity[0].X
			// targetY = entity[0].Y
			// targetZ = entity[0].Z
		}
	}

	if entityToAttack != nil {
		// TO DO - USE BOW OR SWORD DEPENDING ON ENTITY DISTANCE
		g.activateSword()
		//
		if g.calculatePath(entityToAttack) {
			log.Printf("no path found - updating look\n")
			g.updateLook(entityToAttack.X, entityToAttack.Y, entityToAttack.Z)
		}

		if err := g.client.SwingArm(0); err != nil {
			return err
		}

		log.Printf("Attacking Entity: %#v", entityToAttack)
		if err := g.client.AttackEntity(entityToAttack.ID, 0); err != nil { //retrieve
			log.Printf("attack failed: %s\n", err.Error())
			return err
		}
		log.Printf("defend~zombie (prePhysics)\n\n\n")
		time.Sleep(time.Millisecond * 300)
		// if err := g.client.UseItem(0); err != nil { //throw
		// 	return err
		// }
	}

	return nil
}

func (g *defend) calculatePath(entity *world_entity.Entity) bool {
	if entity != nil {
		// g.client.Inputs.Yaw = 0
		g.client.Inputs.Jump = false

		nav := path.Nav{
			World: &g.client.Wd,
			Start: path.V3{X: int(g.client.Pos.X), Y: int(g.client.Pos.Y) - 1, Z: int(g.client.Pos.Z)},
			Dest:  path.V3{X: int(entity.X), Y: int(entity.Y) - 1, Z: int(entity.Z)},
		}

		// if dist(nav.Start, nav.Dest) < 1 {
		// 	log.Printf("near destination not finding path\n")
		// 	g.client.Inputs.ThrottleX = 0
		// 	g.client.Inputs.ThrottleZ = 0
		// 	g.client.Inputs.Jump = false
		// 	return
		// }
		log.Printf("finding path from %#v to %#v\n", nav.Start, nav.Dest)

		thePath, pathDist, found := nav.Path()
		log.Printf("path: %+v\tpathDist: %v\tfound: %v", thePath, pathDist, found)
		if found == false {
			return false
		}

		if len(thePath) > 0 {
			if len(thePath) > 5 {
				thePath = thePath[:5]
			}
			g.currentPath = thePath

			var currentStep astar.Pather
			currentStep, g.currentPath = g.currentPath[len(g.currentPath)-1], g.currentPath[:len(g.currentPath)-1]

			tile := currentStep.(path.Tile)

			for tile.Movement.String() == path.Waypoint.String() {
				if len(g.currentPath) > 0 {
					currentStep, g.currentPath = g.currentPath[len(g.currentPath)-1], g.currentPath[:len(g.currentPath)-1]
					tile = currentStep.(path.Tile)
				} else {
					return false
				}
			}

			g.currentStep = tile
			currentPos := path.Point{X: g.client.Pos.X, Y: g.client.Pos.Y - 1, Z: g.client.Pos.Z}
			currentStepPoint := path.Point{X: float64(g.currentStep.Pos.X), Y: float64(g.currentStep.Pos.Y), Z: float64(g.currentStep.Pos.Z)}
			inputs := getInputs(subPoints(currentPos, currentStepPoint), tile.Movement)

			g.client.Inputs = inputs
			return true
		}

		orgDest := nav.Dest
		for _, m := range path.AllMovements {
			dx, dy, dz := m.Offset()
			newDest := path.V3{X: orgDest.X + dx, Y: orgDest.Y + dy, Z: orgDest.Z + dz}
			nav.Dest = newDest
			thePath, pathDist, found = nav.Path()
			if len(thePath) > 0 {
				g.currentPath = thePath

				var currentStep astar.Pather
				currentStep, g.currentPath = g.currentPath[len(g.currentPath)-1], g.currentPath[:len(g.currentPath)-1]

				tile := currentStep.(path.Tile)
				g.currentStep = tile

				currentPos := path.Point{X: g.client.Pos.X, Y: g.client.Pos.Y - 1, Z: g.client.Pos.Z}
				currentStepPoint := path.Point{X: float64(g.currentStep.Pos.X), Y: float64(g.currentStep.Pos.Y), Z: float64(g.currentStep.Pos.Z)}
				inputs := getInputs(subPoints(currentPos, currentStepPoint), tile.Movement)

				g.client.Inputs = inputs
				return true
			}
		}

		if len(g.currentPath) == 0 {
			log.Printf("Failed to find an alternative starting point\n")
		}

	}
	return false
}

func (g *defend) onSound(params ...interface{}) error {
	// 	/*
	// 	   2021/01/10 17:43:16 Unhandled sound: entity.zombie.step (1446.000000, 74.000000, -860.500000)
	// 	   2021/01/10 17:43:16 Player          Pos: 1446.500000, 74.000000, -860.500000
	// 	   2021/01/10 17:43:16 Unhandled sound: entity.zombie.step (1460.625000, 71.000000, -858.125000)
	// 	   2021/01/10 17:43:16 Player          Pos: 1446.500000, 74.000000, -860.500000
	// 	   2021/01/10 17:43:18 Unhandled sound: entity.zombie.ambient (1445.625000, 74.000000, -860.500000)
	// 	   2021/01/10 17:43:18 Player          Pos: 1446.500000, 74.000000, -860.500000
	// 	   chat received: ROF_bot was slain by Zombie
	// 	*/

	// 	// log.Println("Defend goal handling sound:", params)
	// 	var name string
	// 	// var category int
	// 	// var x, y, z float64
	// 	// var volume, pitch float32

	// 	if len(params) != 7 {
	// 		return fmt.Errorf("Invalid number of parameters to fishing.onSound. got %d, expected 7", len(params))
	// 	}

	// 	name = params[0].(string)
	// 	// category = params[1].(int)
	// 	x := params[2].(float64)
	// 	y := params[3].(float64)
	// 	z := params[4].(float64)
	// 	// volume = params[5].(float32)
	// 	// pitch = params[6].(float32)
	// 	fmt.Printf("%s recieved sound %s\n", g.GetName(), name)
	// 	if strings.Contains(name, "zombie") {
	// 		// if name == "entity.zombie.step" || name == "entity.zombie.ambient" {
	// 		entityList := g.getHostileMobs()
	// 		entityID := int32(0)
	// 		targetX := float64(0)
	// 		targetY := float64(0)
	// 		targetZ := float64(0)
	// 		for _, entity := range entityList {
	// 			if math.Floor(entity.X) == math.Floor(x) && math.Floor(entity.Y) == math.Floor(y) && math.Floor(entity.Z) == math.Floor(z) {
	// 				entityID = entity.ID
	// 				targetX = entity.X
	// 				targetY = entity.Y
	// 				targetZ = entity.Z
	// 				break
	// 			}
	// 		}
	// 		if entityID > 0 {
	// 			g.activateSword()
	// 			g.updateLook(targetX, targetY, targetZ)
	// 			if err := g.client.SwingArm(1); err != nil {
	// 				return err
	// 			}

	// 			if err := g.client.AttackEntity(entityID, 0); err != nil { //retrieve
	// 				log.Printf("attach failed: %s\n", err.Error())
	// 				return err
	// 			}
	// 			log.Println("defend~zombie (sound)")
	// 			// time.Sleep(time.Millisecond * 300)

	// 			// if err := g.client.UseItem(0); err != nil { //throw
	// 			// 	return err
	// 			// }
	// 		} else {
	// 			fmt.Printf("No entity found for sound %s at %f %f %f\n", name, x, y, z)
	// 		}
	// 	}
	// 	// if name == "entity.item.break" {
	// 	// 	if g.client.HeldItem == 0 {
	// 	// 		log.Println("fishing rod broke!")
	// 	// 		g.Stop()
	// 	// 	}
	// 	// }
	return nil

}

// // This should turn the bot towards the x,y,z
// // IT ISN'T CURRENTLY WORKING???? I don't know why :(
func (g *defend) updateLook(x, y, z float64) {
	// x0 := g.client.Pos.X
	// y0 := g.client.Pos.Y
	// z0 := g.client.Pos.Z

	// dx := x - x0
	// dy := y - y0
	// dz := z - z0
	// r := math.Sqrt(dx*dx + dy*dy + dz*dz)
	// yaw := -1 * math.Atan2(dx, dz) / math.Pi * 180
	// if yaw < 0 {
	// 	yaw = 360 + yaw
	// }
	// pitch := -1 * math.Asin(dy/r) / math.Pi * 180

	// fmt.Printf("Updating look yaw: %f \tpitch: %f\n", yaw, pitch)
	// g.client.Pos.Yaw = float32(yaw)
	// g.client.Pos.Pitch = float32(pitch)
	nav := path.Nav{
		World: &g.client.Wd,
		Start: path.V3{X: int(g.client.Pos.X), Y: int(g.client.Pos.Y) - 1, Z: int(g.client.Pos.Z)},
		Dest:  path.V3{X: int(x), Y: int(y) - 1, Z: int(z)},
	}
	thePath, _, _ := nav.Path()

	if /*found == true && */ len(thePath) > 0 {
		next := thePath[0].(path.Tile)
		inputs := next.Inputs(
			path.Point{X: g.client.Pos.X, Y: g.client.Pos.Y, Z: g.client.Pos.Z},
			path.Point{X: x, Y: y, Z: z},
			path.Point{X: 1, Y: 1, Z: 1},
			20*time.Millisecond,
		)

		g.client.Inputs = inputs
	}

}
