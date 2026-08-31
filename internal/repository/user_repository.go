package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"bujo/internal/models"
)

// ErrEmailTaken is returned by Create when the email uniqueness constraint
// rejects the insert. AuthService already checks FindByEmail before calling
// Create, but that check-then-insert has a gap: two registrations for the
// same address arriving close together can both pass the check and race to
// insert, so the constraint — not the pre-check — is the actual last line of
// defense. Without this translation, whichever one lost the race surfaced
// the raw driver error ("duplicate key value violates unique constraint
// ...") straight to the client instead of a message anyone could read.
var ErrEmailTaken = errors.New("email already registered")

// UserRepository is the only thing in the app that knows how a User is
// persisted. Services depend on this interface, never on *gorm.DB directly,
// so the persistence layer can change without touching business logic.
type UserRepository interface {
	Create(user *models.User) error
	FindByEmail(email string) (*models.User, error)
	FindByID(id uint) (*models.User, error)
	Update(user *models.User) error
	List() ([]models.User, error)
	Count() (int64, error)
	CountByRole(role models.Role) (int64, error)
	// DeleteCascade permanently removes the user and everything they own —
	// entries, collections, sessions — in one transaction. This is the only
	// path that deletes a user; both self-service and admin-initiated
	// deletion go through it, so there's exactly one cleanup to keep correct.
	DeleteCascade(user *models.User) error
}

type userRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(user *models.User) error {
	err := r.db.Create(user).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return ErrEmailTaken
	}
	return err
}

func (r *userRepository) FindByEmail(email string) (*models.User, error) {
	var user models.User
	if err := r.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByID(id uint) (*models.User, error) {
	var user models.User
	if err := r.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) List() ([]models.User, error) {
	var users []models.User
	err := r.db.Order("created_at asc").Find(&users).Error
	return users, err
}

func (r *userRepository) Count() (int64, error) {
	var n int64
	err := r.db.Model(&models.User{}).Count(&n).Error
	return n, err
}

func (r *userRepository) CountByRole(role models.Role) (int64, error) {
	var n int64
	err := r.db.Model(&models.User{}).Where("role = ?", role).Count(&n).Error
	return n, err
}

func (r *userRepository) DeleteCascade(user *models.User) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("collection_id IN (?)",
			tx.Model(&models.Collection{}).Select("id").Where("user_id = ?", user.ID),
		).Delete(&models.Entry{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.Entry{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.Collection{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.Session{}).Error; err != nil {
			return err
		}
		return tx.Delete(user).Error
	})
}
