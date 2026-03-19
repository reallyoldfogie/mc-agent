package models

// CustomRegistry provides access to version-agnostic registry lookups.
type CustomRegistry interface {
	GetID() string
	GetNameByID(id int32) (string, bool)
	GetIDByName(name string) (int32, bool)
	IsReady() bool
	Dump()
}
