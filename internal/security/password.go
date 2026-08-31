// Package security holds framework-agnostic, no-DB cryptographic and
// validation helpers — password hashing, email normalization, session
// tokens — shared by services that need them.
package security

import (
	"errors"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost trades a bit of CPU time per login for resistance to offline
// brute-forcing if the password hash table is ever leaked.
const BcryptCost = 12

const MinPasswordLength = 8

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NormalizeEmail lowercases and trims an email so "A@x.com" and "a@x.com "
// are treated as the same account.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidateEmail(email string) error {
	if !emailPattern.MatchString(email) {
		return errors.New("enter a valid email address")
	}
	return nil
}

func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}
