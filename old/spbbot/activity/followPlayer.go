package activity

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"time"

	go_mc_bot "github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/path"
	world_entity "github.com/Tnze/go-mc/bot/world/entity"
	"github.com/beefsack/go-astar"

	"github.com/spbinns/mc-agent/spbbot/types"
	"github.com/spbinns/mc-agent/utils"
)

var (
	players map[string]string = map[string]string{
		"SThomasB_2K7":   "9a677478-0c5b-4aad-a8c3-b03a5235dbac",
		"ReallyOldFogie": "92faf5f0-eb21-4688-a3e5-8d206e56fb72",
		"Thes25":         "4614deac-bcb8-45eb-97d1-9b8eb941a20a",
		"MalificentB":    "cf721358-5f15-49d7-9194-0f9a4778cbc4",
		"ROF_bot":        "a7a0dac9-bbe0-4853-a362-7faf104097a2",
	}
	playersByUUID map[string]string = map[string]string{
		"9a677478-0c5b-4aad-a8c3-b03a5235dbac": "SThomasB_2K7",
		"92faf5f0-eb21-4688-a3e5-8d206e56fb72": "ReallyOldFogie",
		"4614deac-bcb8-45eb-97d1-9b8eb941a20a": "Thes25",
		"cf721358-5f15-49d7-9194-0f9a4778cbc4": "MalificentB",
		"a7a0dac9-bbe0-4853-a362-7faf104097a2": "ROF_bot",
	}
)

const (
	maxPathDist = int(5)
)

type dumpPath struct {
	Start path.V3
	Dest  path.V3
	Path  []path.Tile
}

type followPlayer struct {
	client *go_mc_bot.Client

	calculatingPath bool

	targetedPlayer *world_entity.Entity
	lastCost       float64
	currentPath    []astar.Pather
	currentStep    path.Tile

	running            bool
	stuck              bool
	countSinceLastStep int

	countSinceLastYawAdjustment int
}

// NewFollowPlayer factory
func NewFollowPlayer(client *go_mc_bot.Client) types.Activity {
	return &followPlayer{
		client:      client,
		currentPath: []astar.Pather{},
	}
}

func (g *followPlayer) GetName() string {
	return "Following Player"
}

func (g *followPlayer) Start(keywords []string, command string) error {
	log.Println("Follow player starting: currentPath =", g.currentPath)
	g.running = true

	g.reset()
	g.getNewPath()

	// log.Printf("dumping world to json\n")
	// dump, err := json.Marshal(&g.client.Wd)
	// if err != nil {
	// 	log.Printf("Failed to dump world to json: %v\n", err)

	// 	return err
	// }

	// out, err := os.Create("world_dump.json")
	// if err != nil {
	// 	log.Printf("Failed to open world_jump.json file: %v\n", err)
	// 	return err
	// }
	// defer out.Close()

	// _, err = out.Write(dump)
	// if err != nil {
	// 	log.Printf("Failed to write to world_jump.json file: %v\n", err)
	// 	return err
	// }

	return nil
}

func (g *followPlayer) Stop() error {
	log.Println("Follow player stopping")
	g.reset()
	return nil
}

func (g *followPlayer) IsRunning() bool {
	return g.running
}

func (g *followPlayer) reset() {
	g.currentPath = []astar.Pather{}
	g.client.Inputs.ThrottleX = 0
	g.client.Inputs.ThrottleZ = 0
	g.client.Inputs.Jump = false
	g.running = false
	g.stuck = false
	g.countSinceLastStep = 0
	g.countSinceLastYawAdjustment = 0

}

func (g *followPlayer) RegisterCallbacks() map[string]func(params ...interface{}) error {
	return map[string]func(params ...interface{}) error{
		"PrePhysicsCallback": g.prePhysicsCallback,
	}
}

func (g *followPlayer) getNewPath() error {
	if len(g.currentPath) == 0 && g.calculatingPath == false {
		// path.V3{X:1443, Y:75, Z:-854}
		// targetedPlayer, cost := &world_entity.Entity{
		// 	X: 1443, Y: 75, Z: -854,
		// }, float64(0)

		targetedPlayer, cost := utils.FindNearestPlayer(g.client.Pos, g.client.Wd.PlayerEntities())
		if targetedPlayer == nil {
			g.Stop()
			return g.client.Chat("No players found to follow")
		}

		g.calculatingPath = true
		g.lastCost = cost
		g.targetedPlayer = targetedPlayer

		go g.calculatePath()
	}
	return nil
}

func (g *followPlayer) calculatePath() {
	targetedPlayer := g.targetedPlayer
	if targetedPlayer != nil {
		// g.client.Inputs.Yaw = 0
		g.client.Inputs.Jump = false

		nav := path.Nav{
			World: &g.client.Wd,
			Start: path.V3{X: int(g.client.Pos.X), Y: int(g.client.Pos.Y) - 1, Z: int(g.client.Pos.Z)},
			Dest:  path.V3{X: int(targetedPlayer.X), Y: int(targetedPlayer.Y) - 1, Z: int(targetedPlayer.Z)},
		}

		if dist(nav.Start, nav.Dest) < 1 {
			log.Printf("near destination not finding path\n")
			g.client.Inputs.ThrottleX = 0
			g.client.Inputs.ThrottleZ = 0
			g.client.Inputs.Jump = false
			g.calculatingPath = false

			return
		}
		log.Printf("finding path from %#v to %#v\n", nav.Start, nav.Dest)
		// straightLine := utils.Line(nav.Start, nav.Dest)

		thePath, pathDist, found := nav.Path()
		log.Printf("path: %+v\tpathDist: %v\tfound: %v", thePath, pathDist, found)

		if len(thePath) > 0 {
			if len(thePath) > 5 {
				thePath = thePath[:5]
			}
			g.currentPath = thePath

			var currentStep astar.Pather
			currentStep, g.currentPath = g.currentPath[len(g.currentPath)-1], g.currentPath[:len(g.currentPath)-1]

			tile := currentStep.(path.Tile)
			g.currentStep = tile
			g.countSinceLastYawAdjustment--
		} else {
			// if g.countSinceLastYawAdjustment <= 0 {
			// 	g.countSinceLastYawAdjustment = 10
			// 	oldYaw := g.client.Inputs.Yaw
			// 	// rotate 90 degrees in preparation for the next check
			// 	g.client.Inputs.Yaw += 7.5
			// 	if g.client.Inputs.Yaw > 179.9 {
			// 		g.client.Inputs.Yaw = -180.0
			// 	}

			// 	log.Printf("Yaw changed: %v => %v\n", oldYaw, g.client.Inputs.Yaw)
			// }
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
					g.countSinceLastYawAdjustment--
					break
				}
			}

			if len(g.currentPath) == 0 {
				log.Printf("Failed to find an alternative starting point\n")
			}
		}
	} else {
		g.client.Chat("say stop following")
	}

	g.calculatingPath = false
}

func dist(src path.V3, dest path.V3) float64 {
	x, y, z := src.X-dest.X, src.Y-dest.Y, src.Z-dest.Z
	return math.Sqrt(float64(x*x + z*z + y*y)) // return euclydean distance
}

func (g *followPlayer) prePhysicsCallback(params ...interface{}) error {
	if g.calculatingPath == false {
		// if len(curPath) <= 0 {
		if len(g.currentPath) <= 0 {
			g.client.Inputs = path.Inputs{}

			return g.getNewPath()
		}

		pos := g.client.Player.Pos

	start:
		next := g.currentPath[len(g.currentPath)-1].(path.Tile)

		dx, dy, dz := pos.X-float64(next.Pos.X)-0.48, pos.Y-float64(next.Pos.Y)-1, pos.Z-float64(next.Pos.Z)-0.48
		if next.IsComplete(path.Point{X: dx, Y: dy, Z: dz}) {
			// fmt.Printf("next path marker is %s\n", next.Pos)
			g.currentPath = g.currentPath[:len(g.currentPath)-1]
			if len(g.currentPath) > maxPathDist {
				goto start
			} else {
				// if useBedOnDest {
				// 	// bed(int(pos.X+dx), int(pos.Y+dy), int(pos.Z+dz))
				// 	bed(bedX, bedY, bedZ)
				// 	useBedOnDest = false
				// }
			}
		}

		inputs := next.Inputs(
			path.Point{X: pos.X, Y: pos.Y, Z: pos.Z},
			path.Point{X: dx, Y: dy, Z: dz},
			path.Point{X: 1, Y: 1, Z: 1},
			20*time.Millisecond,
		)

		g.client.Inputs = inputs
	}
	return nil

}
func (g *followPlayer) prePhysicsCallbackX(params ...interface{}) error {
	if g.calculatingPath == false {
		if len(g.currentPath) > 0 || g.stuck {
			currentPos := path.Point{X: g.client.Pos.X, Y: g.client.Pos.Y - 1, Z: g.client.Pos.Z}
			currentStepPoint := path.Point{X: float64(g.currentStep.Pos.X), Y: float64(g.currentStep.Pos.Y), Z: float64(g.currentStep.Pos.Z)}
			log.Println("currentPos:", currentPos, "currentStepPoint", currentStepPoint, "g.currentStep.Pos", g.currentStep.Pos)
			log.Printf("\n\n%d == %d && %d == %d && %d == %d\n%#v\ncountSinceLastStep = %v\nstuck = %v\n\n",
				int(currentPos.X), g.currentStep.Pos.X, int(currentPos.Y), g.currentStep.Pos.Y, int(currentPos.Z), g.currentStep.Pos.Z,
				g.client.Inputs,
				g.countSinceLastStep,
				g.stuck,
			)

			if (int(currentPos.X) == g.currentStep.Pos.X && int(currentPos.Y) == g.currentStep.Pos.Y && int(currentPos.Z) == g.currentStep.Pos.Z) ||
				(g.client.Inputs.ThrottleX == 0 && g.client.Inputs.ThrottleZ == 0) ||
				g.stuck == true {
				log.Println("PrePhysicsCallback currentPath:", g.currentPath)

				g.countSinceLastStep = 0
				g.stuck = false

				var currentStep astar.Pather
				currentStep, g.currentPath = g.currentPath[len(g.currentPath)-1], g.currentPath[:len(g.currentPath)-1]
				tile := currentStep.(path.Tile)
				for tile.Movement.String() == path.Waypoint.String() {
					if len(g.currentPath) > 0 {
						currentStep, g.currentPath = g.currentPath[len(g.currentPath)-1], g.currentPath[:len(g.currentPath)-1]
						tile = currentStep.(path.Tile)
					} else {
						return g.getNewPath()
					}
				}
				g.currentStep = tile
				currentStepPoint = path.Point{X: float64(g.currentStep.Pos.X), Y: float64(g.currentStep.Pos.Y), Z: float64(g.currentStep.Pos.Z)}
				// if tile.Movement != path.Waypoint {
				// inputs := tile.Inputs(currentPos, subPoints(currentPos, currentStepPoint), path.Point{X: 0, Y: 0, Z: 0}, 0*time.Second)
				inputs := getInputs(subPoints(currentPos, currentStepPoint), tile.Movement)

				log.Printf("inputs: %#v\n", inputs)
				// g.client.Chat(fmt.Sprintf("/setblock %d %d %d bedrock\n", tile.Pos.X, tile.Pos.Y, tile.Pos.Z))
				// log.Printf("setblock %d %d %d bedrock\n", tile.Pos.X, tile.Pos.Y, tile.Pos.Z)
				g.client.Inputs = inputs
				// }
			} else {
				g.countSinceLastStep++
				if g.countSinceLastStep >= 5 {
					if g.client.Inputs.Jump == true {
						g.stuck = true
					} else {
						g.client.Inputs.Jump = true
					}
				}
			}
			if int(currentPos.X) == g.currentStep.Pos.X {
				g.client.Inputs.ThrottleX = 0
			}
			if int(currentPos.Z) == g.currentStep.Pos.Z {
				g.client.Inputs.ThrottleZ = 0
			}

		} else {
			// g.client.Chat("say stop following")
			g.client.Inputs.ThrottleX = 0
			g.client.Inputs.ThrottleZ = 0
			return g.getNewPath()
		}
	}
	return nil
}

func getInputs(deltaPos path.Point, m path.Movement) path.Inputs {
	log.Println("getInputs(\"", deltaPos, "\"", m.String(), "\")")

	// Sufficient for simple movements.
	at := math.Atan2(-deltaPos.X, -deltaPos.Z)
	mdX, mdY, mdZ := m.Offset()
	wantYaw := -math.Atan2(float64(mdX), float64(mdZ)) * 180 / math.Pi
	out := path.Inputs{
		ThrottleX: math.Sin(at),
		ThrottleZ: math.Cos(at),
		Yaw:       wantYaw,
	}
	if mdX == 0 && mdZ == 0 {
		out.Yaw = math.NaN()
	}
	if (rand.Int() % 14) == 0 {
		out.Pitch = float64((rand.Int() % 4) - 2)
	}

	if mdY > 0 {
		out.Jump = true
	}

	return out
}

func subPoints(src, dest path.Point) path.Point {
	return path.Point{
		X: src.X - dest.X,
		Y: src.Y - dest.Y,
		Z: src.Z - dest.Z,
	}
}

func dump(obj interface{}) {
	out, err := json.Marshal(&obj)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal object (%#v): %v\n", obj, err)
		return
	}

	log.Println(string(out))

}
