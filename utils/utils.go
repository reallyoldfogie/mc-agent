package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"

	data_entity "github.com/Tnze/go-mc/data/entity"
	"github.com/reallyoldfogie/mc-agent/bot/path"
	"github.com/reallyoldfogie/mc-agent/bot/world/entity"
	"github.com/reallyoldfogie/mc-agent/bot/world/entity/player"
	"github.com/reallyoldfogie/mc-agent/models"
)

// FindPlayer -
func FindPlayer(list []entity.Entity, userUUID string) *entity.Entity {
	// fmt.Println(list)
	for _, ent := range list {
		// dumpEntity(&ent)
		if ent.UUID.String() == userUUID {
			return &ent
		}
	}
	return nil
}

// FindNearestPlayer -
func FindNearestPlayer(self player.Pos, list []entity.Entity) (*entity.Entity, float64) {
	var nearestPlayer *entity.Entity
	nearestDist := math.MaxFloat64
	startPos := path.V3{X: int(self.X), Y: int(self.Y), Z: int(self.Z)}
	for _, ent := range list {
		playerPos := path.V3{X: int(ent.X), Y: int(ent.Y), Z: int(ent.Z)}
		cost := startPos.Cost(playerPos)
		nearestDist = math.Min(cost, nearestDist)
		if nearestDist == cost {
			nearestPlayer = &ent
		}
	}
	return nearestPlayer, nearestDist
}

// FindEntity -
func FindEntity(list map[int32]*entity.Entity, name string) *entity.Entity {
	for _, ent := range list {
		if ent.Base.DisplayName == name || ent.Base.Name == name {
			return ent
		}
	}
	return nil
}

// DumpEntity ...
func DumpEntity(ent *entity.Entity) {
	empJSON, err := json.MarshalIndent(ent, "", "  ")
	if err != nil {
		fmt.Println("ERROR: Failed to marshal entity: ", err.Error())
		return
	}

	fmt.Println("entityJson:" + string(empJSON))
}

// GetYawAndPitch calculates yaw and pitch to look from src to dest.
func GetYawAndPitch(src, dest models.V3) (yaw, pitch float64) {
	delta := dest.Sub(src)
	distanceFromSrcToDest := src.DistanceTo(dest)

	yaw = -math.Atan2(delta.X, delta.Z) / math.Pi * 180
	pitch = -math.Asin(delta.Y/distanceFromSrcToDest) / math.Pi * 180

	log.Printf("GetYawAndPitch: src=%s, dest=%s, delta=%s, r=%.2f => yaw=%.2f, pitch=%.2f", src, dest, delta, distanceFromSrcToDest, yaw, pitch)
	return
}

// FindNearestFish -
func FindNearestFish(self player.Pos, list []entity.Entity) (*entity.Entity, float64) {
	var nearestFish *entity.Entity
	nearestDist := math.MaxFloat64
	startPos := path.V3{X: int(self.X), Y: int(self.Y), Z: int(self.Z)}
	for _, ent := range list {
		if dataEntity, ok := data_entity.ByID[data_entity.ID(ent.ID)]; ok {
			if strings.Contains(dataEntity.Name, "fish") {
				fishPos := path.V3{X: int(ent.X), Y: int(ent.Y), Z: int(ent.Z)}
				cost := startPos.Cost(fishPos)
				nearestDist = math.Min(cost, nearestDist)
				if nearestDist == cost {
					nearestFish = &ent
				}
			}
		}
	}
	return nearestFish, nearestDist

}
