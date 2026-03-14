package models

// Auth mirrors the authentication details required by the underlying client.
type Auth struct {
	AccessToken string
	Name        string
	UUID        string
}
