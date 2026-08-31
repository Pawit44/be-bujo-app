package repository

import (
	"errors"

	"gorm.io/gorm"

	"bujo/internal/models"
)

var ErrCollectionNotFound = errors.New("collection not found")

type CollectionRepository interface {
	Create(col *models.Collection) error
	// FindOwned loads a collection only if it belongs to userID — an id
	// that exists but belongs to someone else returns ErrCollectionNotFound,
	// identical to an id that doesn't exist at all.
	FindOwned(id, userID uint) (*models.Collection, error)
	// IsOwnedBy reports whether id belongs to userID, without loading the
	// row — used to validate a collectionId referenced from an entry.
	IsOwnedBy(id, userID uint) (bool, error)
	List(userID uint) ([]models.Collection, error)
	Update(col *models.Collection) error
	Delete(col *models.Collection) error
	CountForUser(userID uint) (int64, error)
	MaxPosition(userID uint) (*int, error)
}

type collectionRepository struct{ db *gorm.DB }

func NewCollectionRepository(db *gorm.DB) CollectionRepository {
	return &collectionRepository{db: db}
}

func (r *collectionRepository) Create(col *models.Collection) error {
	return r.db.Create(col).Error
}

func (r *collectionRepository) FindOwned(id, userID uint) (*models.Collection, error) {
	var col models.Collection
	if err := r.db.Where("user_id = ?", userID).First(&col, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCollectionNotFound
		}
		return nil, err
	}
	return &col, nil
}

func (r *collectionRepository) IsOwnedBy(id, userID uint) (bool, error) {
	var count int64
	err := r.db.Model(&models.Collection{}).Where("id = ? AND user_id = ?", id, userID).Count(&count).Error
	return count > 0, err
}

func (r *collectionRepository) List(userID uint) ([]models.Collection, error) {
	var collections []models.Collection
	err := r.db.Where("user_id = ?", userID).Order("pinned desc, position asc, id asc").Find(&collections).Error
	return collections, err
}

func (r *collectionRepository) Update(col *models.Collection) error {
	return r.db.Save(col).Error
}

// Delete removes the collection, every entry that lived on it, and every
// folder it had — a folder only ever means something as part of its
// collection, so it has no reason to survive the collection itself.
func (r *collectionRepository) Delete(col *models.Collection) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("collection_id = ?", col.ID).Delete(&models.Entry{}).Error; err != nil {
			return err
		}
		if err := tx.Where("collection_id = ?", col.ID).Delete(&models.Folder{}).Error; err != nil {
			return err
		}
		return tx.Delete(col).Error
	})
}

func (r *collectionRepository) CountForUser(userID uint) (int64, error) {
	var n int64
	err := r.db.Model(&models.Collection{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

func (r *collectionRepository) MaxPosition(userID uint) (*int, error) {
	var max *int
	err := r.db.Model(&models.Collection{}).Where("user_id = ?", userID).Select("MAX(position)").Scan(&max).Error
	return max, err
}
