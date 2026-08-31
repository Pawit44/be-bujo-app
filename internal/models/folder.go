package models

import (
	"time"

	"gorm.io/gorm"
)

// Folder is a single, flat level of grouping inside one Collection — a
// named bucket entries can optionally sit in. Folders don't nest: a folder
// belongs to a collection, never to another folder, which is deliberate —
// this is meant to split a collection's entries into a few labeled groups,
// not to be a general-purpose filesystem.
type Folder struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	UserID       uint           `gorm:"index;not null" json:"-"`
	CollectionID uint           `gorm:"index;not null" json:"collectionId"`
	Title        string         `gorm:"not null" json:"title"`
	Position     int            `gorm:"default:0" json:"position"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Folder) TableName() string { return "folders" }
