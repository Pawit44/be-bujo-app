package service

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"bujo/internal/limits"
	"bujo/internal/models"
	"bujo/internal/repository"
)

var (
	ErrFolderNotFound      = repository.ErrFolderNotFound
	ErrFolderTitleRequired = errors.New("title is required")
)

type FolderInput struct {
	Title    string
	Position *int
}

type FolderService struct {
	folders     repository.FolderRepository
	collections repository.CollectionRepository
}

func NewFolderService(folders repository.FolderRepository, collections repository.CollectionRepository) *FolderService {
	return &FolderService{folders: folders, collections: collections}
}

// List returns every folder in a collection — the caller must already have
// verified it owns that collection; this trusts the collectionID it's given
// and only additionally scopes by userID as defense in depth.
func (s *FolderService) List(collectionID, userID uint) ([]models.Folder, error) {
	return s.folders.ListByCollection(collectionID, userID)
}

func (s *FolderService) Create(collectionID, userID uint, in FolderInput) (*models.Folder, error) {
	if _, err := s.collections.FindOwned(collectionID, userID); err != nil {
		return nil, err
	}
	if in.Title == "" {
		return nil, ErrFolderTitleRequired
	}
	if err := validateFolderTitle(in.Title); err != nil {
		return nil, err
	}

	count, err := s.folders.CountForCollection(collectionID)
	if err != nil {
		return nil, err
	}
	if count >= limits.MaxFoldersPerCollection {
		return nil, fmt.Errorf("folder limit reached (%d) — delete one before adding more", limits.MaxFoldersPerCollection)
	}

	folder := &models.Folder{UserID: userID, CollectionID: collectionID, Title: in.Title}
	if in.Position != nil {
		folder.Position = *in.Position
	} else {
		max, err := s.folders.MaxPosition(collectionID)
		if err != nil {
			return nil, err
		}
		if max != nil {
			folder.Position = *max + 1
		}
	}

	if err := s.folders.Create(folder); err != nil {
		return nil, err
	}
	return folder, nil
}

func (s *FolderService) Update(id, userID uint, in FolderInput) (*models.Folder, error) {
	folder, err := s.folders.FindOwned(id, userID)
	if err != nil {
		return nil, err
	}
	if in.Title != "" {
		if err := validateFolderTitle(in.Title); err != nil {
			return nil, err
		}
		folder.Title = in.Title
	}
	if in.Position != nil {
		folder.Position = *in.Position
	}
	if err := s.folders.Update(folder); err != nil {
		return nil, err
	}
	return folder, nil
}

// Delete removes the folder; its entries move back to the collection's
// unsorted area (see repository.Delete) rather than being deleted with it.
func (s *FolderService) Delete(id, userID uint) error {
	folder, err := s.folders.FindOwned(id, userID)
	if err != nil {
		return err
	}
	return s.folders.Delete(folder)
}

func validateFolderTitle(title string) error {
	if utf8.RuneCountInString(title) > limits.MaxFolderTitleLength {
		return fmt.Errorf("title must be %d characters or fewer", limits.MaxFolderTitleLength)
	}
	return nil
}
