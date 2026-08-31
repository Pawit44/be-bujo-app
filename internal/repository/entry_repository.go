package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"bujo/internal/models"
)

var ErrEntryNotFound = errors.New("entry not found")

// EntryFilters narrows an entry List — every field is optional (zero value
// means "don't filter on this").
type EntryFilters struct {
	LogKind      models.LogKind
	Month        string
	Date         string
	DateFrom     string
	DateTo       string
	CollectionID string // raw query param; empty means unset
	Status       string
	Type         string
	Search       string
}

type EntryRepository interface {
	Create(entry *models.Entry) error
	// FindOwned loads an entry only if it belongs to userID — an id that
	// exists but belongs to someone else returns ErrEntryNotFound, exactly
	// like an id that doesn't exist, so this can't be used to probe ids.
	FindOwned(id, userID uint) (*models.Entry, error)
	List(userID uint, filters EntryFilters) ([]models.Entry, error)
	Update(entry *models.Entry) error
	Delete(entry *models.Entry) error
	DeleteByCollectionID(collectionID uint) error
	CountForUser(userID uint) (int64, error)
	CountByCollection(collectionID uint, status models.EntryStatus) (int64, error)
	// CountOwnedAmong reports how many of ids belong to userID — used to
	// verify a whole batch (e.g. a drag-reorder) before touching any of it.
	CountOwnedAmong(ids []uint, userID uint) (int64, error)
	NextPosition(userID uint, logKind models.LogKind, month, date string, collectionID *uint) (int, error)
	// UpdatePositions sets position = index in orderedIDs, for every id, in
	// one transaction — the persistence-layer half of a drag-reorder.
	UpdatePositions(userID uint, orderedIDs []uint) error
	// RecentForUser returns the most recently touched entries that belong to
	// the current month: a weekly entry dated inside it, a monthly/future
	// entry for it, or any collection entry (those aren't date-scoped, so
	// they're always eligible). Older entries some past action happened to
	// touch again are deliberately excluded — that's what the Review page is
	// for; this list is meant to answer "what have I been doing this month,"
	// not surface stale months next to today's work.
	RecentForUser(userID uint, limit int, month, monthStart, monthEnd string) ([]models.Entry, error)
	// ListDue returns every open entry whose spread has already passed —
	// the BuJo migration ritual's raw material: a weekly entry dated before
	// today, a monthly entry from a month that's over, or a future entry
	// whose month has arrived (future-log items are meant to be migrated
	// into the monthly/weekly log once their month starts, not left sitting
	// in Future). Each log kind measures "past" on the field it actually
	// uses — comparing every row to the same date/month would either miss
	// month-scoped rows entirely or misjudge day-scoped ones.
	ListDue(userID uint, today, month string) ([]models.Entry, error)
	// Scoped returns a query already filtered to userID, for the read-only
	// reporting aggregates the Index/Stats pages need (assorted counts by
	// status/type/date-range). Reporting queries are inherently varied in
	// shape; a fixed-signature method per combination would multiply
	// endlessly, so this is a deliberate, narrow escape hatch — used only
	// by OverviewService, never for writes, and every caller still starts
	// from a user-scoped base, so multi-tenancy can't be forgotten.
	Scoped(userID uint) *gorm.DB
}

type entryRepository struct{ db *gorm.DB }

func NewEntryRepository(db *gorm.DB) EntryRepository {
	return &entryRepository{db: db}
}

func (r *entryRepository) Create(entry *models.Entry) error {
	return r.db.Create(entry).Error
}

func (r *entryRepository) FindOwned(id, userID uint) (*models.Entry, error) {
	var entry models.Entry
	if err := r.db.Where("user_id = ?", userID).First(&entry, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEntryNotFound
		}
		return nil, err
	}
	return &entry, nil
}

func (r *entryRepository) List(userID uint, f EntryFilters) ([]models.Entry, error) {
	q := r.db.Model(&models.Entry{}).Where("user_id = ?", userID)

	if f.LogKind != "" {
		q = q.Where("log_kind = ?", f.LogKind)
	}
	if f.Month != "" {
		q = q.Where("month = ?", f.Month)
	}
	if f.Date != "" {
		q = q.Where("date = ?", f.Date)
	}
	if f.DateFrom != "" && f.DateTo != "" {
		q = q.Where("date >= ? AND date <= ?", f.DateFrom, f.DateTo)
	}
	if f.CollectionID != "" {
		q = q.Where("collection_id = ?", f.CollectionID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.Search != "" {
		like := "%" + f.Search + "%"
		q = q.Where("(content ILIKE ? OR notes ILIKE ?)", like, like)
	}

	var entries []models.Entry
	err := q.Order("position asc, id asc").Find(&entries).Error
	return entries, err
}

func (r *entryRepository) Update(entry *models.Entry) error {
	return r.db.Save(entry).Error
}

func (r *entryRepository) Delete(entry *models.Entry) error {
	return r.db.Delete(entry).Error
}

func (r *entryRepository) DeleteByCollectionID(collectionID uint) error {
	return r.db.Where("collection_id = ?", collectionID).Delete(&models.Entry{}).Error
}

func (r *entryRepository) CountForUser(userID uint) (int64, error) {
	var n int64
	err := r.db.Model(&models.Entry{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

func (r *entryRepository) CountByCollection(collectionID uint, status models.EntryStatus) (int64, error) {
	q := r.db.Model(&models.Entry{}).Where("collection_id = ?", collectionID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

func (r *entryRepository) CountOwnedAmong(ids []uint, userID uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var n int64
	err := r.db.Model(&models.Entry{}).Where("id IN ? AND user_id = ?", ids, userID).Count(&n).Error
	return n, err
}

func (r *entryRepository) NextPosition(userID uint, logKind models.LogKind, month, date string, collectionID *uint) (int, error) {
	q := r.db.Model(&models.Entry{}).Where("user_id = ? AND log_kind = ?", userID, logKind)
	switch logKind {
	case models.LogFuture, models.LogMonthly:
		q = q.Where("month = ?", month)
	case models.LogWeekly:
		q = q.Where("date = ?", date)
	case models.LogCollection:
		q = q.Where("collection_id = ?", collectionID)
	}
	var max *int
	if err := q.Select("MAX(position)").Scan(&max).Error; err != nil {
		return 0, err
	}
	if max == nil {
		return 0, nil
	}
	return *max + 1, nil
}

// UpdatePositions writes every new position in a single statement. A loop of
// one UPDATE per row meant a dragged entry cost as many database round trips
// as there are entries in the list; a CASE expression collapses that to one,
// and the `user_id` guard keeps the batch scoped to its owner exactly as the
// per-row version did.
func (r *entryRepository) UpdatePositions(userID uint, orderedIDs []uint) error {
	if len(orderedIDs) == 0 {
		return nil
	}

	// The THEN values are bound parameters, which Postgres would otherwise
	// infer as text and then refuse to assign to a bigint column, so each one
	// is cast explicitly.
	var cases strings.Builder
	caseArgs := make([]any, 0, len(orderedIDs)*2)
	cases.WriteString("CASE id")
	for i, id := range orderedIDs {
		cases.WriteString(" WHEN ? THEN CAST(? AS bigint)")
		caseArgs = append(caseArgs, id, i)
	}
	cases.WriteString(" END")

	return r.db.Model(&models.Entry{}).
		Where("id IN ? AND user_id = ?", orderedIDs, userID).
		Update("position", gorm.Expr(cases.String(), caseArgs...)).Error
}

func (r *entryRepository) RecentForUser(userID uint, limit int, month, monthStart, monthEnd string) ([]models.Entry, error) {
	var entries []models.Entry
	err := r.db.Where("user_id = ?", userID).
		Where(
			"log_kind = ? OR (log_kind IN (?, ?) AND month = ?) OR (log_kind = ? AND date >= ? AND date <= ?)",
			models.LogCollection,
			models.LogMonthly, models.LogFuture, month,
			models.LogWeekly, monthStart, monthEnd,
		).
		Order("updated_at desc").Limit(limit).Find(&entries).Error
	return entries, err
}

func (r *entryRepository) ListDue(userID uint, today, month string) ([]models.Entry, error) {
	var entries []models.Entry
	err := r.db.Where("user_id = ? AND status = ?", userID, models.StatusOpen).
		Where(
			"(log_kind = ? AND date < ?) OR (log_kind = ? AND month < ?) OR (log_kind = ? AND month <= ?)",
			models.LogWeekly, today,
			models.LogMonthly, month,
			models.LogFuture, month,
		).
		Order("date asc, month asc, position asc, id asc").
		Find(&entries).Error
	return entries, err
}

func (r *entryRepository) Scoped(userID uint) *gorm.DB {
	return r.db.Model(&models.Entry{}).Where("user_id = ?", userID)
}
