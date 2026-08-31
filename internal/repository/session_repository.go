package repository

import (
	"errors"

	"gorm.io/gorm"

	"bujo/internal/models"
)

var ErrSessionNotFound = errors.New("session not found")

type SessionRepository interface {
	Create(session *models.Session) error
	FindByTokenHash(hash string) (*models.Session, error)
	Update(session *models.Session) error
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

func (r *sessionRepository) Update(session *models.Session) error {
	return r.db.Save(session).Error
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
