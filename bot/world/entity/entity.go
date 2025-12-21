package entity

import (
	// TODO: REPLACE RELIANCE ON go-mc/data/entity - it is version-locked and out of date
	//       Need version-agnostic EntityMgr (similar to BlockMgr and ItemMgr):
	//       1. Create EntityMgr interface in mc-protocol-go/data/versions
	//       2. Generate version-specific entity data files in mc-data-gen
	//       3. EntityMgr should provide:
	//          - GetEntityByID(id int) -> EntityType with Name, Width, Height, Category, etc.
	//          - GetEntityByName(name string) -> EntityType
	//          - Version-specific entity registry mapping
	//          - Entity metadata field types and parsers
	//       4. Replace entity.Entity references with entityMgr methods
	//       This will enable proper entity handling across Minecraft versions
	"github.com/Tnze/go-mc/data/entity"
	"github.com/google/uuid"
)

// BlockEntity describes the representation of a tile entity at a position.
type BlockEntity struct {
	ID string `nbt:"id"`

	// global co-ordinates
	X int `nbt:"x"`
	Y int `nbt:"y"`
	Z int `nbt:"z"`

	// sign-specific.
	Color string `nbt:"color"`
	Text1 string `nbt:"Text1"`
	Text2 string `nbt:"Text2"`
	Text3 string `nbt:"Text3"`
	Text4 string `nbt:"Text4"`
}

// Entity represents an instance of an entity.
type Entity struct {
	ID   int32
	Data int32
	Base *entity.Entity

	UUID uuid.UUID

	X, Y, Z          float64
	Pitch, Yaw       int8
	VelX, VelY, VelZ int16
	OnGround         bool

	IsLiving  bool
	HeadPitch int8
}

// // The Slot data structure is how Minecraft represents an item and its associated data in the Minecraft Protocol
// type Slot struct {
// 	Present bool
// 	ItemID  item.ID
// 	Count   int8
// 	NBT     interface{}
// }

// //Decode implement packet.FieldDecoder interface
// func (s *Slot) Decode(r pk.DecodeReader) error {
// 	if err := (*pk.Boolean)(&s.Present).Decode(r); err != nil {
// 		return err
// 	}
// 	if s.Present {
// 		var itemID pk.VarInt
// 		if err := itemID.Decode(r); err != nil {
// 			return err
// 		}
// 		s.ItemID = item.ID(itemID)
// 		if err := (*pk.Byte)(&s.Count).Decode(r); err != nil {
// 			return err
// 		}
// 		if _,err := nbt.NewDecoder(r).Decode(&s.NBT); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func (s Slot) Encode() []byte {
// 	if !s.Present {
// 		return pk.Boolean(false).Encode()
// 	}

// 	var b bytes.Buffer
// 	b.Write(pk.Boolean(true).Encode())
// 	b.Write(pk.VarInt(s.ItemID).Encode())
// 	b.Write(pk.Byte(s.Count).Encode())

// 	if s.NBT != nil {
// 		nbt.NewEncoder(&b).Encode(s.NBT)
// 	} else {
// 		b.Write([]byte{nbt.TagEnd})
// 	}

// 	return b.Bytes()
// }

// func (s Slot) String() string {
// 	return item.ByID[item.ID(s.ItemID)].DisplayName
// }
