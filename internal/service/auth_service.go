package service

import (
	"errors"
	"strings"
	"time"

	"bujo/internal/models"
	"bujo/internal/repository"
	"bujo/internal/security"
)

const (
	// SessionTTL is how long a session stays valid without use. Each
	// successful validation slides the expiry forward.
	SessionTTL = 30 * 24 * time.Hour

	maxFailedLogins = 5
	lockoutDuration = 15 * time.Minute

	maxNameLength = 120
)

var (
	ErrEmailTaken       = errors.New("an account with this email already exists")
	ErrInvalidLogin     = errors.New("incorrect email or password")
	ErrAccountLocked    = errors.New("too many failed attempts — try again later")
	ErrInvalidSession   = errors.New("invalid or expired session")
	ErrWrongPassword    = errors.New("incorrect password")
	ErrLastAdminAccount = errors.New("you're the last admin — promote someone else first")
)

// A precomputed bcrypt hash with no corresponding real password, used only
// to burn constant-ish time when an email lookup misses, so the login
// endpoint can't be used to discover which emails have accounts.
const dummyHashForTiming = "$2a$12$uLSFqpjEJvd2LY54WojZFu1Uy3uBJRndK0ieHqm75xfZ5B4xBMrcS"

type AuthService struct {
	users       repository.UserRepository
	sessions    repository.SessionRepository
	adminEmails map[string]struct{}
}

func NewAuthService(users repository.UserRepository, sessions repository.SessionRepository, adminEmails map[string]struct{}) *AuthService {
	return &AuthService{users: users, sessions: sessions, adminEmails: adminEmails}
}

func (s *AuthService) isAdminEmail(email string) bool {
	_, ok := s.adminEmails[email]
	return ok
}

// Register creates a plain user account — admin is never granted by
// registration order, only by a matching email in the admin allowlist (or
// later promotion via the admin panel).
func (s *AuthService) Register(email, password, name string) (*models.User, string, time.Time, error) {
	email = security.NormalizeEmail(email)
	if err := security.ValidateEmail(email); err != nil {
		return nil, "", time.Time{}, err
	}
	if err := security.ValidatePassword(password); err != nil {
		return nil, "", time.Time{}, err
	}
	name = strings.TrimSpace(name)
	if len(name) > maxNameLength {
		name = name[:maxNameLength]
	}

	if _, err := s.users.FindByEmail(email); err == nil {
		return nil, "", time.Time{}, ErrEmailTaken
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	role := models.RoleUser
	if s.isAdminEmail(email) {
		role = models.RoleAdmin
	}

	user := &models.User{Email: email, PasswordHash: hash, Name: name, Role: role}
	if err := s.users.Create(user); err != nil {
		return nil, "", time.Time{}, err
	}

	token, expiresAt, err := s.issueSession(user.ID)
	return user, token, expiresAt, err
}

// Login validates credentials and, on success, issues a new session.
// Failures are deliberately generic and identical whether the email
// doesn't exist or the password is wrong.
func (s *AuthService) Login(email, password string) (*models.User, string, time.Time, error) {
	email = security.NormalizeEmail(email)

	user, err := s.users.FindByEmail(email)
	if err != nil {
		// Run a real bcrypt comparison anyway so this branch costs roughly
		// the same time as the real-password path.
		security.CheckPassword(dummyHashForTiming, password)
		return nil, "", time.Time{}, ErrInvalidLogin
	}

	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		return nil, "", time.Time{}, ErrAccountLocked
	}

	if !security.CheckPassword(user.PasswordHash, password) {
		user.FailedLoginAttempts++
		if user.FailedLoginAttempts >= maxFailedLogins {
			lockUntil := time.Now().Add(lockoutDuration)
			user.LockedUntil = &lockUntil
			user.FailedLoginAttempts = 0
		}
		_ = s.users.Update(user)
		return nil, "", time.Time{}, ErrInvalidLogin
	}

	if user.FailedLoginAttempts != 0 || user.LockedUntil != nil {
		user.FailedLoginAttempts = 0
		user.LockedUntil = nil
		_ = s.users.Update(user)
	}

	token, expiresAt, err := s.issueSession(user.ID)
	return user, token, expiresAt, err
}

// ValidateSession resolves a raw cookie token to its user, sliding the
// session's expiry forward so an active user is never logged out mid-use.
func (s *AuthService) ValidateSession(token string) (*models.User, error) {
	if token == "" {
		return nil, ErrInvalidSession
	}
	session, err := s.sessions.FindByTokenHash(security.HashToken(token))
	if err != nil {
		return nil, ErrInvalidSession
	}
	if time.Now().After(session.ExpiresAt) {
		_ = s.sessions.Delete(session)
		return nil, ErrInvalidSession
	}

	user, err := s.users.FindByID(session.UserID)
	if err != nil {
		return nil, ErrInvalidSession
	}

	session.ExpiresAt = time.Now().Add(SessionTTL)
	_ = s.sessions.Update(session)

	return user, nil
}

func (s *AuthService) Logout(token string) {
	if token == "" {
		return
	}
	_ = s.sessions.DeleteByTokenHash(security.HashToken(token))
}

// DeleteAccount lets a user erase their own account and everything they
// wrote, with no admin involved — the self-service side of the right to
// erasure. Requires re-entering the current password so a hijacked,
// unlocked session can't wipe an account silently.
func (s *AuthService) DeleteAccount(user *models.User, password string) error {
	if !security.CheckPassword(user.PasswordHash, password) {
		return ErrWrongPassword
	}
	if user.Role == models.RoleAdmin {
		admins, err := s.users.CountByRole(models.RoleAdmin)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdminAccount
		}
	}
	return s.users.DeleteCascade(user)
}

func (s *AuthService) issueSession(userID uint) (string, time.Time, error) {
	token, err := security.GenerateSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(SessionTTL)
	session := &models.Session{TokenHash: security.HashToken(token), UserID: userID, ExpiresAt: expiresAt}
	if err := s.sessions.Create(session); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}
