package service

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"bujo/internal/models"
	"bujo/internal/repository"
)

var ErrInvalidMonth = errors.New("month must look like YYYY-MM")

type LogSummary struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Total int64  `json:"total"`
	Open  int64  `json:"open"`
	Done  int64  `json:"done"`
}

type MonthSummary struct {
	Month string `json:"month"`
	Total int64  `json:"total"`
	Open  int64  `json:"open"`
	Done  int64  `json:"done"`
}

type IndexView struct {
	Today        string              `json:"today"`
	Month        string              `json:"month"`
	WeekStart    string              `json:"weekStart"`
	WeekEnd      string              `json:"weekEnd"`
	Logs         []LogSummary        `json:"logs"`
	FutureMonths []MonthSummary      `json:"futureMonths"`
	Collections  []models.Collection `json:"collections"`
	Recent       []models.Entry      `json:"recent"`
	Totals       struct {
		Entries int64 `json:"entries"`
		Done    int64 `json:"done"`
	} `json:"totals"`
}

type StatsView struct {
	Month          string           `json:"month"`
	Total          int64            `json:"total"`
	ByStatus       map[string]int64 `json:"byStatus"`
	ByType         map[string]int64 `json:"byType"`
	CompletionRate float64          `json:"completionRate"`
}

type OverviewService struct {
	entries     repository.EntryRepository
	collections repository.CollectionRepository
}

func NewOverviewService(entries repository.EntryRepository, collections repository.CollectionRepository) *OverviewService {
	return &OverviewService{entries: entries, collections: collections}
}

// Index gathers everything the INDEX page needs in one round trip.
func (s *OverviewService) Index(userID uint) (*IndexView, error) {
	now := time.Now()
	month := now.Format("2006-01")
	weekStart, weekEnd := weekBounds(now)

	base := s.entries.Scoped(userID)

	view := &IndexView{
		Today:     now.Format("2006-01-02"),
		Month:     month,
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
	}

	view.Logs = []LogSummary{
		summaryFor("future", "Future Log", base.Session(&gorm.Session{}).Where("log_kind = ?", models.LogFuture)),
		summaryFor("monthly", "Monthly Log", base.Session(&gorm.Session{}).Where("log_kind = ? AND month = ?", models.LogMonthly, month)),
		summaryFor("weekly", "Weekly Log", base.Session(&gorm.Session{}).Where("log_kind = ? AND date >= ? AND date <= ?", models.LogWeekly, weekStart, weekEnd)),
	}

	// The next six months of the future log, so the index can preview them.
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	view.FutureMonths = make([]MonthSummary, 0, 6)
	for i := 0; i < 6; i++ {
		m := monthStart.AddDate(0, i, 0).Format("2006-01")
		scoped := base.Session(&gorm.Session{}).Where("log_kind = ? AND month = ?", models.LogFuture, m)
		s := summaryFor(m, m, scoped)
		view.FutureMonths = append(view.FutureMonths, MonthSummary{Month: m, Total: s.Total, Open: s.Open, Done: s.Done})
	}

	collections, err := s.collections.List(userID)
	if err != nil {
		return nil, err
	}
	view.Collections = collections

	recent, err := s.entries.RecentForUser(userID, 8)
	if err != nil {
		return nil, err
	}
	view.Recent = recent

	if err := base.Session(&gorm.Session{}).Count(&view.Totals.Entries).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Where("status = ?", models.StatusDone).Count(&view.Totals.Done).Error; err != nil {
		return nil, err
	}

	return view, nil
}

// Stats returns completion figures for one month.
func (s *OverviewService) Stats(userID uint, month string) (*StatsView, error) {
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	start := month + "-01"
	t, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, ErrInvalidMonth
	}
	end := t.AddDate(0, 1, -1).Format("2006-01-02")

	scope := s.entries.Scoped(userID).Where("(month = ? OR (date >= ? AND date <= ?))", month, start, end)

	byStatus := map[string]int64{}
	for _, st := range []models.EntryStatus{models.StatusOpen, models.StatusDone, models.StatusMigrated, models.StatusScheduled, models.StatusCancelled} {
		var n int64
		if err := scope.Session(&gorm.Session{}).Where("status = ?", st).Count(&n).Error; err != nil {
			return nil, err
		}
		byStatus[string(st)] = n
	}

	byType := map[string]int64{}
	for _, ty := range []models.EntryType{models.TypeTask, models.TypeEvent, models.TypeNote} {
		var n int64
		if err := scope.Session(&gorm.Session{}).Where("type = ?", ty).Count(&n).Error; err != nil {
			return nil, err
		}
		byType[string(ty)] = n
	}

	var total int64
	if err := scope.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}

	rate := 0.0
	if total > 0 {
		rate = float64(byStatus[string(models.StatusDone)]) / float64(total) * 100
	}

	return &StatsView{Month: month, Total: total, ByStatus: byStatus, ByType: byType, CompletionRate: rate}, nil
}

func summaryFor(key, label string, base *gorm.DB) LogSummary {
	s := LogSummary{Key: key, Label: label}
	base.Session(&gorm.Session{}).Count(&s.Total)
	base.Session(&gorm.Session{}).Where("status = ?", models.StatusOpen).Count(&s.Open)
	base.Session(&gorm.Session{}).Where("status = ?", models.StatusDone).Count(&s.Done)
	return s
}

// weekBounds returns the Monday and Sunday around t as YYYY-MM-DD.
func weekBounds(t time.Time) (string, string) {
	offset := (int(t.Weekday()) + 6) % 7 // Monday = 0
	start := t.AddDate(0, 0, -offset)
	return start.Format("2006-01-02"), start.AddDate(0, 0, 6).Format("2006-01-02")
}
