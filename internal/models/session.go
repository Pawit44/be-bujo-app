package models

import "time"

// Session backs a login. Only a hash of the token is stored, so a stolen
// database dump alone can't be replayed as a valid session cookie.
type Session struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	TokenHash string    `gorm:"uniqueIndex;not null" json:"-"`
	UserID    uint      `gorm:"index;not null" json:"-"`
	ExpiresAt time.Time `gorm:"index;not null" json:"-"`
	CreatedAt time.Time `json:"-"`
}

func (Session) TableName() string { return "sessions" }
