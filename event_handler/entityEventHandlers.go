package eventHandler

//"github.com/mattn/go-colorable"

//"github.com/Tnze/go-mc/bot"

// soundid_1_20_3 "github.com/reallyoldfogie/mc-agent/data/soundid/1.20.3"
// soundid_1_21_1 "github.com/reallyoldfogie/mc-agent/data/soundid/1.21.1"
// soundid_1_21_3 "github.com/reallyoldfogie/mc-agent/data/soundid/1.21.3"

// func HandleEntityPositionPacket(c *bot.Client, p pk.Packet) error {
// 	var pu ptypes.EntityPosition
// 	if err := pu.Decode(p); err != nil {
// 		return err
// 	}
// 	return c.Wd.OnEntityPosUpdate(pu)
// }

// func HandleEntityPositionLookPacket(c *bot.Client, p pk.Packet) error {
// 	var epr ptypes.EntityPositionLook
// 	if err := epr.Decode(p); err != nil {
// 		return err
// 	}
// 	return c.Wd.OnEntityPosLookUpdate(epr)
// }

// func HandleEntityLookPacket(c *bot.Client, p pk.Packet) error {
// 	var er ptypes.EntityRotation
// 	if err := er.Decode(p); err != nil {
// 		return err
// 	}
// 	return c.Wd.OnEntityLookUpdate(er)
// }

// func HandleEntityMovePacket(c *bot.Client, p pk.Packet) error {
// 	var id pk.VarInt
// 	if err := p.Scan(&id); err != nil {
// 		return err
// 	}
// 	fmt.Printf("EntityMove (probs didnt for players): %+v\n", id)
// 	return nil
// }

// func HandleEntityAnimationPacket(c *bot.Client, p pk.Packet) error {
// 	var se ptypes.EntityAnimationClientbound
// 	if err := se.Decode(p); err != nil {
// 		return err
// 	}
// 	// fmt.Printf("EntityAnimationClientbound: %+v\n", se)
// 	return nil
// }

// func HandleEntityStatusPacket(c *bot.Client, p pk.Packet) error {
// 	var (
// 		id     pk.Int
// 		status pk.Byte
// 	)
// 	if err := p.Scan(&id, &status); err != nil {
// 		return err
// 	}
// 	// fmt.Printf("EntityStatus: %v, %v\n", id, status)
// 	return nil
// }

// func HandleDestroyEntitiesPacket(c *bot.Client, p pk.Packet) error {
// 	var (
// 		count pk.VarInt
// 		r     = bytes.NewReader(p.Data)
// 	)
// 	if err := count.Decode(r); err != nil {
// 		return err
// 	}

// 	entities := make([]pk.VarInt, int(count))
// 	for i := 0; i < int(count); i++ {
// 		if err := entities[i].Decode(r); err != nil {
// 			return err
// 		}
// 	}

// 	return c.Wd.OnEntityDestroy(entities)
// }
