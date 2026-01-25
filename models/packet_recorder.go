package models

// PacketRecorder is a minimal recorder interface used for replay capture.
type PacketRecorder interface {
	RecordNow(id int32, payload []byte) error
	SetSelfID(id int)
	AddPlayer(uuid string)
}
