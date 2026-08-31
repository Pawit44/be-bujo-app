package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"bujo/internal/models"
)

var ErrSessionNotFound = errors.New("session not found")

type SessionRepository interface {
	Create(session *models.Session) error
	FindByTokenHash(hash string) (*models.Session, error)
	// FindWithUserByTokenHash resolves a token hash to its session and the
	// user that owns it in one query. Every authenticated request pays this
	// lookup, so it is deliberately a join rather than two round trips.
	FindWithUserByTokenHash(hash string) (*models.Session, *models.User, error)
	Update(session *models.Session) error
	// TouchExpiry slides one session's expiry without loading or rewriting
	// the whole row.
	TouchExpiry(id uint, expiresAt time.Time) error
	Delete(session *models.Session) error
	DeleteByTokenHash(hash string) error
	DeleteAllForUser(userID uint) error
}

type sessionRepository struct{ db *gorm.DB }

func NewSessionRepository(db *gorm.DB) SessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) Create(session *models.Session) error {
	return r.db.Create(session).Error
}

func (r *sessionRepository) FindByTokenHash(hash string) (*models.Session, error) {
	var session models.Session
	if err := r.db.Where("token_hash = ?", hash).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

// FindWithUserByTokenHash joins sessions to users so a request's identity
// costs one query. A soft-deleted user yields ErrSessionNotFound — the same
// answer as a token that never existed.
func (r *sessionRepository) FindWithUserByTokenHash(hash string) (*models.Session, *models.User, error) {
	var row struct {
		models.Session
		User models.User `gorm:"embedded;embeddedPrefix:u_"`
	}
	err := r.db.Model(&models.Session{}).
		Select("sessions.*, "+
			"users.id AS u_id, users.email AS u_email, users.password_hash AS u_password_hash, "+
			"users.name AS u_name, users.role AS u_role, users.failed_login_attempts AS u_failed_login_attempts, "+
			"users.locked_until AS u_locked_until, users.created_at AS u_created_at, users.updated_at AS u_updated_at").
		Joins("JOIN users ON users.id = sessions.user_id AND users.deleted_at IS NULL").
		Where("sessions.token_hash = ?", hash).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrSessionNotFound
		}
		return nil, nil, err
	}
	session := row.Session
	user := row.User
	return &session, &user, nil
}

func (r *sessionRepository) Update(session *models.Session) error {
	return r.db.Save(session).Error
}

func (r *sessionRepository) TouchExpiry(id uint, expiresAt time.Time) error {
	return r.db.Model(&models.Session{}).Where("id = ?", id).Update("expires_at", expiresAt).Error
}

func (r *sessionRepository) Delete(session *models.Session) error {
	return r.db.Delete(session).Error
}

func (r *sessionRepository) DeleteByTokenHash(hash string) error {
	return r.db.Where("token_hash = ?", hash).Delete(&models.Session{}).Error
}

func (r *sessionRepository) DeleteAllForUser(userID uint) error {
	return r.db.Where("user_id = ?", userID).Delete(&models.Session{}).Error
}
