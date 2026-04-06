package agent

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/reallyoldfogie/mc-agent/models"
)

type containerEvents struct {
	agent models.Agent
}

func (ce containerEvents) Open(id int, containerType int32, title chat.Message) error { return nil }
func (ce containerEvents) SetSlot(id int, index int16) error {
	return ce.agent.OnScreenSlotChange(id, index)
}
func (ce containerEvents) Close(code int) error { return nil }
