package models

// ItemManager provides item name lookups by ID.
type ItemManager interface {
	GetItemNameByID(id int) string
}
