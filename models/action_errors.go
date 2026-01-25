package models

import "errors"

// ErrActionNotFound signals a missing action.
var ErrActionNotFound = errors.New("action not found")
