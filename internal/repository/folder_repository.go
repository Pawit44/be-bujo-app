package repository

import (
	"errors"

	"gorm.io/gorm"

	"bujo/internal/models"
)

var ErrFolderNotFound = errors.New("folder not found")

type FolderRepository interface {
	Create(folder *models.Folder) error
	// FindOwned loads a folder only if it belongs to userID — an id that
	// exists but belongs to someone else returns ErrFolderNotFound, identical
	// to an id that doesn't exist at all.
	FindOwned(id, userID uint) (*models.Folder, error)
	ListByCollection(collectionID, userID uint) ([]models.Folder, error)
	Update(folder *models.Folder) error
	// Delete removes the folder and clears FolderID on every entry that was
	// in it — those entries move back to the collection's unsorted area
	// rather than being deleted themselves. Deleting a folder is tidying up
	// a grouping, not throwing away what was grouped.
	Delete(folder *models.Folder) error
	CountForCollection(collectionID uint) (int64, error)
	MaxPosition(collectionID uint) (*int, error)
}

type folderRepository struct{ db *gorm.DB }

func NewFolderRepository(db *gorm.DB) FolderRepository {
	return &folderRepository{db: db}
}

func (r *folderRepository) Create(folder *models.Folder) error {
	return r.db.Create(folder).Error
}

func (r *folderRepository) FindOwned(id, userID uint) (*models.Folder, error) {
	var folder models.Folder
	if err := r.db.Where("user_id = ?", userID).First(&folder, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFolderNotFound
		}
		return nil, err
	}
	return &folder, nil
}

func (r *folderRepository) ListByCollection(collectionID, userID uint) ([]models.Folder, error) {
	var folders []models.Folder
	err := r.db.Where("collection_id = ? AND user_id = ?", collectionID, userID).
		Order("position asc, id asc").
		Find(&folders).Error
	return folders, err
}

func (r *folderRepository) Update(folder *models.Folder) error {
	return r.db.Save(folder).Error
}

func (r *folderRepository) Delete(folder *models.Folder) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Entry{}).
			Where("folder_id = ?", folder.ID).
			Update("folder_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(folder).Error
	})
}

func (r *folderRepository) CountForCollection(collectionID uint) (int64, error) {
	var n int64
	err := r.db.Model(&models.Folder{}).Where("collection_id = ?", collectionID).Count(&n).Error
	return n, err
}

func (r *folderRepository) MaxPosition(collectionID uint) (*int, error) {
	var max *int
	err := r.db.Model(&models.Folder{}).Where("collection_id = ?", collectionID).Select("MAX(position)").Scan(&max).Error
	return max, err
}
