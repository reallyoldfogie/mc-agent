package models

// Errors
type configError string

func (e configError) Error() string     { return string(e) }
func ErrInvalidConfig(msg string) error { return configError(msg) }
