package models

import (
	"time"

	"gorm.io/gorm"
)

// Collection is a named spread listed on the INDEX page.
type Collection struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	UserID      uint           `gorm:"index;not null;default:0" json:"-"`
	Title       string         `gorm:"not null" json:"title"`
	Description string         `json:"description"`
	Color       string         `gorm:"default:'slate'" json:"color"`
	Icon        string         `gorm:"default:'book'" json:"icon"`
	Pinned      bool           `gorm:"default:false" json:"pinned"`
	Position    int            `gorm:"default:0" json:"position"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Entries []Entry `gorm:"foreignKey:CollectionID" json:"entries,omitempty"`
}

func (Collection) TableName() string { return "collections" }
