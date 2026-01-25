package models

// ChatOperations provides chat functionality for the agent.
type ChatOperations interface {
	SendChat(message string) error
}
