package models

// ChatOperations provides chat functionality for the agent.
type ChatOperations interface {
	SendChat(message string) error
	// SendCommand sends a server command through the agent's player
	// connection. command must not include the leading slash.
	SendCommand(command string) error
}
