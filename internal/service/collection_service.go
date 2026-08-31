package service

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"bujo/internal/limits"
	"bujo/internal/models"
	"bujo/internal/repository"
)

var ErrTitleRequired = errors.New("title is required")

// CollectionInput carries every field a collection create/update can touch.
type CollectionInput struct {
	Title       string
	Description *string
	Color       string
	Icon        string
	Pinned      *bool
	Position    *int
}

// CollectionView is a collection plus the progress counters the INDEX and
// Collections pages show.
type CollectionView struct {
	models.Collection
	Total int64 `json:"total"`
	Open  int64 `json:"open"`
	Done  int64 `json:"done"`
}

type CollectionService struct {
	collections repository.CollectionRepository
	entries     repository.EntryRepository
}

func NewCollectionService(collections repository.CollectionRepository, entries repository.EntryRepository) *CollectionService {
	return &CollectionService{collections: collections, entries: entries}
}

// List returns every collection for userID with its progress counters.
func (s *CollectionService) List(userID uint) ([]CollectionView, error) {
	collections, err := s.collections.List(userID)
	if err != nil {
		return nil, err
	}

	views := make([]CollectionView, 0, len(collections))
	for _, col := range collections {
		total, err := s.entries.CountByCollection(col.ID, "")
		if err != nil {
			return nil, err
		}
		open, err := s.entries.CountByCollection(col.ID, models.StatusOpen)
		if err != nil {
			return nil, err
		}
		done, err := s.entries.CountByCollection(col.ID, models.StatusDone)
		if err != nil {
			return nil, err
		}
		views = append(views, CollectionView{Collection: col, Total: total, Open: open, Done: done})
	}
	return views, nil
}

// Get returns a collection with its entries populated.
func (s *CollectionService) Get(id, userID uint) (*models.Collection, error) {
	col, err := s.collections.FindOwned(id, userID)
	if err != nil {
		return nil, err
	}
	entries, err := s.entries.List(userID, repository.EntryFilters{CollectionID: fmt.Sprint(col.ID)})
	if err != nil {
		return nil, err
	}
	col.Entries = entries
	return col, nil
}

func (s *CollectionService) Create(userID uint, in CollectionInput) (*models.Collection, error) {
	if in.Title == "" {
		return nil, ErrTitleRequired
	}
	if err := validateCollectionText(in.Title, in.Description); err != nil {
		return nil, err
	}

	count, err := s.collections.CountForUser(userID)
	if err != nil {
		return nil, err
	}
	if count >= limits.MaxCollectionsPerUser {
		return nil, fmt.Errorf("collection limit reached (%d) — delete one before adding more", limits.MaxCollectionsPerUser)
	}

	col := &models.Collection{UserID: userID, Title: in.Title, Color: in.Color, Icon: in.Icon}
	if col.Color == "" {
		col.Color = "slate"
	}
	if col.Icon == "" {
		col.Icon = "book"
	}
	if in.Description != nil {
		col.Description = *in.Description
	}
	if in.Pinned != nil {
		col.Pinned = *in.Pinned
	}
	if in.Position != nil {
		col.Position = *in.Position
	} else {
		max, err := s.collections.MaxPosition(userID)
		if err != nil {
			return nil, err
		}
		if max != nil {
			col.Position = *max + 1
		}
	}

	if err := s.collections.Create(col); err != nil {
		return nil, err
	}
	return col, nil
}

func (s *CollectionService) Update(id, userID uint, in CollectionInput) (*models.Collection, error) {
	col, err := s.collections.FindOwned(id, userID)
	if err != nil {
		return nil, err
	}
	if err := validateCollectionText(in.Title, in.Description); err != nil {
		return nil, err
	}

	if in.Title != "" {
		col.Title = in.Title
	}
	if in.Description != nil {
		col.Description = *in.Description
	}
	if in.Color != "" {
		col.Color = in.Color
	}
	if in.Icon != "" {
		col.Icon = in.Icon
	}
	if in.Pinned != nil {
		col.Pinned = *in.Pinned
	}
	if in.Position != nil {
		col.Position = *in.Position
	}

	if err := s.collections.Update(col); err != nil {
		return nil, err
	}
	return col, nil
}

// Delete removes the collection and every entry that lived on it.
func (s *CollectionService) Delete(id, userID uint) error {
	col, err := s.collections.FindOwned(id, userID)
	if err != nil {
		return err
	}
	return s.collections.Delete(col)
}

// validateCollectionText bounds how much text a collection's title and
// description can hold.
func validateCollectionText(title string, description *string) error {
	if utf8.RuneCountInString(title) > limits.MaxCollectionTitleLength {
		return fmt.Errorf("title must be %d characters or fewer", limits.MaxCollectionTitleLength)
	}
	if description != nil && utf8.RuneCountInString(*description) > limits.MaxCollectionDescriptionLength {
		return fmt.Errorf("description must be %d characters or fewer", limits.MaxCollectionDescriptionLength)
	}
	return nil
}
