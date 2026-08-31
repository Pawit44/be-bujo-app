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
	ErrEntryNotFound      = repository.ErrEntryNotFound
	ErrCollectionNotFound = repository.ErrCollectionNotFound
	ErrContentRequired    = errors.New("content is required")
	ErrLogKindRequired    = errors.New("logKind is required")
)

// EntryInput carries every field an entry create/update can touch. Pointer
// fields mean "leave unchanged" when nil on update; on create, nil means
// "use the default."
type EntryInput struct {
	Content      string
	Type         models.EntryType
	Status       models.EntryStatus
	LogKind      models.LogKind
	Month        string
	Date         string
	CollectionID *uint
	Priority     *bool
	Inspiration  *bool
	Position     *int
	Notes        *string
}

// MigrateInput is where an entry is being moved to.
type MigrateInput struct {
	LogKind      models.LogKind
	Month        string
	Date         string
	CollectionID *uint
}

type EntryService struct {
	entries     repository.EntryRepository
	collections repository.CollectionRepository
}

func NewEntryService(entries repository.EntryRepository, collections repository.CollectionRepository) *EntryService {
	return &EntryService{entries: entries, collections: collections}
}

func (s *EntryService) List(userID uint, filters repository.EntryFilters) ([]models.Entry, error) {
	return s.entries.List(userID, filters)
}

func (s *EntryService) Get(id, userID uint) (*models.Entry, error) {
	return s.entries.FindOwned(id, userID)
}

func (s *EntryService) Create(userID uint, in EntryInput) (*models.Entry, error) {
	if in.Content == "" {
		return nil, ErrContentRequired
	}
	if in.LogKind == "" {
		return nil, ErrLogKindRequired
	}
	if err := s.checkOwnsCollection(userID, in.CollectionID); err != nil {
		return nil, err
	}
	if err := validateEntryText(in.Content, in.Notes); err != nil {
		return nil, err
	}
	if err := s.checkEntryQuota(userID); err != nil {
		return nil, err
	}

	entry := &models.Entry{
		UserID:       userID,
		Content:      in.Content,
		Type:         orDefault(in.Type, models.TypeTask),
		Status:       orDefaultStatus(in.Status, models.StatusOpen),
		LogKind:      in.LogKind,
		Month:        in.Month,
		Date:         in.Date,
		CollectionID: in.CollectionID,
	}
	if in.Priority != nil {
		entry.Priority = *in.Priority
	}
	if in.Inspiration != nil {
		entry.Inspiration = *in.Inspiration
	}
	if in.Notes != nil {
		entry.Notes = *in.Notes
	}
	if in.Position != nil {
		entry.Position = *in.Position
	} else {
		pos, err := s.entries.NextPosition(userID, entry.LogKind, entry.Month, entry.Date, entry.CollectionID)
		if err != nil {
			return nil, err
		}
		entry.Position = pos
	}

	if err := s.entries.Create(entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *EntryService) Update(id, userID uint, in EntryInput) (*models.Entry, error) {
	entry, err := s.entries.FindOwned(id, userID)
	if err != nil {
		return nil, err
	}
	if err := validateEntryText(in.Content, in.Notes); err != nil {
		return nil, err
	}

	if in.Content != "" {
		entry.Content = in.Content
	}
	if in.Type != "" {
		entry.Type = in.Type
	}
	if in.Status != "" {
		entry.Status = in.Status
	}
	if in.LogKind != "" {
		if err := s.checkOwnsCollection(userID, in.CollectionID); err != nil {
			return nil, err
		}
		entry.LogKind = in.LogKind
		entry.Month = in.Month
		entry.Date = in.Date
		entry.CollectionID = in.CollectionID
	} else {
		if in.Month != "" {
			entry.Month = in.Month
		}
		if in.Date != "" {
			entry.Date = in.Date
		}
		if in.CollectionID != nil {
			if err := s.checkOwnsCollection(userID, in.CollectionID); err != nil {
				return nil, err
			}
			entry.CollectionID = in.CollectionID
		}
	}
	if in.Priority != nil {
		entry.Priority = *in.Priority
	}
	if in.Inspiration != nil {
		entry.Inspiration = *in.Inspiration
	}
	if in.Position != nil {
		entry.Position = *in.Position
	}
	if in.Notes != nil {
		entry.Notes = *in.Notes
	}

	if err := s.entries.Update(entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// Toggle flips a task between open and done.
func (s *EntryService) Toggle(id, userID uint) (*models.Entry, error) {
	entry, err := s.entries.FindOwned(id, userID)
	if err != nil {
		return nil, err
	}
	if entry.Status == models.StatusDone {
		entry.Status = models.StatusOpen
	} else {
		entry.Status = models.StatusDone
	}
	if err := s.entries.Update(entry); err != nil {
		return nil, err
	}
	return entry, nil
}

var (
	// ErrAlreadyMigrated: once moved, that's final for this record — it
	// already points at where it went, so it can't be migrated again.
	ErrAlreadyMigrated = errors.New("this entry has already been migrated")
	// ErrSameLocation: migrating to the location an entry already lives in
	// would just create a duplicate sitting next to itself.
	ErrSameLocation = errors.New("already there — nothing to migrate")
)

// Migrate marks the original as migrated (>) or scheduled (<) and recreates
// it on the destination spread — the core bullet journal migration ritual.
// Enforced here, not just in the UI: an entry can't be migrated to the
// location it's already in, and an entry that's already migrated/scheduled
// can't be migrated a second time.
func (s *EntryService) Migrate(id, userID uint, in MigrateInput) (source *models.Entry, migrated *models.Entry, err error) {
	entry, err := s.entries.FindOwned(id, userID)
	if err != nil {
		return nil, nil, err
	}
	if in.LogKind == "" {
		return nil, nil, ErrLogKindRequired
	}
	if entry.Status == models.StatusMigrated || entry.Status == models.StatusScheduled {
		return nil, nil, ErrAlreadyMigrated
	}
	if isSameLocation(entry, in) {
		return nil, nil, ErrSameLocation
	}
	if err := s.checkOwnsCollection(userID, in.CollectionID); err != nil {
		return nil, nil, err
	}
	if err := s.checkEntryQuota(userID); err != nil {
		return nil, nil, err
	}

	moved := &models.Entry{
		UserID:       userID,
		Content:      entry.Content,
		Type:         entry.Type,
		Status:       models.StatusOpen,
		LogKind:      in.LogKind,
		Month:        in.Month,
		Date:         in.Date,
		CollectionID: in.CollectionID,
		Priority:     entry.Priority,
		Inspiration:  entry.Inspiration,
		Notes:        entry.Notes,
	}
	pos, err := s.entries.NextPosition(userID, moved.LogKind, moved.Month, moved.Date, moved.CollectionID)
	if err != nil {
		return nil, nil, err
	}
	moved.Position = pos
	if err := s.entries.Create(moved); err != nil {
		return nil, nil, err
	}

	// Pushing something into the future log is "scheduled" (<); every
	// other destination is a plain "migrated" (>).
	if in.LogKind == models.LogFuture {
		entry.Status = models.StatusScheduled
	} else {
		entry.Status = models.StatusMigrated
	}
	if err := s.entries.Update(entry); err != nil {
		return nil, nil, err
	}

	return entry, moved, nil
}

// isSameLocation reports whether target is exactly where entry already
// lives.
func isSameLocation(entry *models.Entry, target MigrateInput) bool {
	if entry.LogKind != target.LogKind {
		return false
	}
	switch target.LogKind {
	case models.LogWeekly:
		return entry.Date == target.Date
	case models.LogMonthly, models.LogFuture:
		return entry.Month == target.Month
	case models.LogCollection:
		if entry.CollectionID == nil || target.CollectionID == nil {
			return entry.CollectionID == nil && target.CollectionID == nil
		}
		return *entry.CollectionID == *target.CollectionID
	default:
		return false
	}
}

var ErrEntriesNotOwned = errors.New("one or more entries not found")

// Reorder persists a drag-and-drop ordering. Every id must belong to the
// caller — otherwise a guessed id from another account could be silently
// adopted into this reorder.
func (s *EntryService) Reorder(userID uint, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	owned, err := s.entries.CountOwnedAmong(ids, userID)
	if err != nil {
		return err
	}
	if int(owned) != len(ids) {
		return ErrEntriesNotOwned
	}
	return s.entries.UpdatePositions(userID, ids)
}

func (s *EntryService) Delete(id, userID uint) error {
	entry, err := s.entries.FindOwned(id, userID)
	if err != nil {
		return err
	}
	return s.entries.Delete(entry)
}

func (s *EntryService) checkOwnsCollection(userID uint, collectionID *uint) error {
	if collectionID == nil {
		return nil
	}
	owned, err := s.collections.IsOwnedBy(*collectionID, userID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrCollectionNotFound
	}
	return nil
}

func (s *EntryService) checkEntryQuota(userID uint) error {
	count, err := s.entries.CountForUser(userID)
	if err != nil {
		return err
	}
	if count >= limits.MaxEntriesPerUser {
		return fmt.Errorf("entry limit reached (%d) — delete some entries before adding more", limits.MaxEntriesPerUser)
	}
	return nil
}

// validateEntryText bounds how much text one entry can hold, so a single
// row can't grow the database without limit. notes is a pointer since it's
// only being changed when present in the request.
func validateEntryText(content string, notes *string) error {
	if utf8.RuneCountInString(content) > limits.MaxEntryContentLength {
		return fmt.Errorf("content must be %d characters or fewer", limits.MaxEntryContentLength)
	}
	if notes != nil && utf8.RuneCountInString(*notes) > limits.MaxEntryNotesLength {
		return fmt.Errorf("notes must be %d characters or fewer", limits.MaxEntryNotesLength)
	}
	return nil
}

func orDefault(v, def models.EntryType) models.EntryType {
	if v == "" {
		return def
	}
	return v
}

func orDefaultStatus(v, def models.EntryStatus) models.EntryStatus {
	if v == "" {
		return def
	}
	return v
}
