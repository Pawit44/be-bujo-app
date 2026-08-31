package models

import (
	"time"

	"gorm.io/gorm"
)

// Role is a user's access level. Admins manage accounts; they do not get
// implicit access to other users' journal content — that stays private.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// User is an account. Passwords are never stored in plain text — only a
// bcrypt hash — and are never serialized to JSON.
type User struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	Email               string         `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash        string         `gorm:"not null" json:"-"`
	Name                string         `json:"name"`
	Role                Role           `gorm:"type:varchar(20);default:'user';index;not null" json:"role"`
	FailedLoginAttempts int            `gorm:"default:0" json:"-"`
	LockedUntil         *time.Time     `json:"-"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

func (User) TableName() string { return "users" }
