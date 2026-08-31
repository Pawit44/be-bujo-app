package models

import (
	"time"

	"gorm.io/gorm"
)

// EntryType is the bullet journal "bullet" kind.
//
//	task  -> •   event -> ○   note -> —
type EntryType string

const (
	TypeTask  EntryType = "task"
	TypeEvent EntryType = "event"
	TypeNote  EntryType = "note"
)

// EntryStatus is the state of a bullet.
//
//	open -> •   done -> X   migrated -> >   scheduled -> <   cancelled -> strike
type EntryStatus string

const (
	StatusOpen      EntryStatus = "open"
	StatusDone      EntryStatus = "done"
	StatusMigrated  EntryStatus = "migrated"
	StatusScheduled EntryStatus = "scheduled"
	StatusCancelled EntryStatus = "cancelled"
)

// LogKind tells which spread an entry lives on.
type LogKind string

const (
	LogFuture     LogKind = "future"     // uses Month (YYYY-MM)
	LogMonthly    LogKind = "monthly"    // uses Month (YYYY-MM)
	LogWeekly     LogKind = "weekly"     // uses Date (day inside the week)
	LogCollection LogKind = "collection" // uses CollectionID
)

// Entry is a single bullet: task, event or note.
type Entry struct {
	ID      uint        `gorm:"primaryKey" json:"id"`
	UserID  uint        `gorm:"index;not null;default:0" json:"-"`
	Content string      `gorm:"not null" json:"content"`
	Type    EntryType   `gorm:"type:varchar(20);default:'task';index" json:"type"`
	Status  EntryStatus `gorm:"type:varchar(20);default:'open';index" json:"status"`

	LogKind LogKind `gorm:"type:varchar(20);index;not null" json:"logKind"`
	// Month is "YYYY-MM" for future / monthly logs.
	Month string `gorm:"index" json:"month"`
	// Date is "YYYY-MM-DD" for weekly logs (the day the bullet belongs to).
	Date string `gorm:"index" json:"date"`

	CollectionID *uint `gorm:"index" json:"collectionId"`
	// FolderID optionally places a collection entry inside one of that
	// collection's folders. Only meaningful when CollectionID is set — an
	// entry outside a collection has no folder to belong to. Nil means the
	// entry sits in the collection's own unsorted area, not inside any folder.
	FolderID *uint `gorm:"index" json:"folderId"`

	Priority    bool `gorm:"default:false" json:"priority"`    // *
	Inspiration bool `gorm:"default:false" json:"inspiration"` // !
	Position    int  `gorm:"default:0" json:"position"`

	Notes     string         `json:"notes"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Entry) TableName() string { return "entries" }
