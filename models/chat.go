package models

// Chat provides a way to send messages to the server.
type Chat interface {
	SendMessage(string) error
}
