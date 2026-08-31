package service

import (
	"errors"
	"sync"
	"time"

	"bujo/internal/clock"
	"bujo/internal/models"
	"bujo/internal/repository"
)

// futureMonthsAhead is how many months of the future log the index previews.
const futureMonthsAhead = 6

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
	// DueForReview is how many open entries have a spread in the past —
	// the count behind the sidebar's Review badge. The list itself is
	// fetched separately (GET /entries/review) only when that page opens.
	DueForReview int64 `json:"dueForReview"`
	Totals       struct {
		Entries int64 `json:"entries"`
		Done    int64 `json:"done"`
	} `json:"totals"`
	// TypeBreakdown is every entry, all-time, split the same way the app's
	// own entry-list tabs split them — task/event/note/idea, each with an
	// open and a done count — for the Profile page's "what's actually in
	// here" drill-down. Always these four tabs in this order, even at zero,
	// so the frontend never has to guess which tabs exist.
	TypeBreakdown []TypeCount `json:"typeBreakdown"`
}

type TypeCount struct {
	Tab  string `json:"tab"`
	Open int64  `json:"open"`
	Done int64  `json:"done"`
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

// indexCounts holds every scalar count the index needs, filled by a single
// conditional-aggregation query instead of one COUNT round trip per figure.
type indexCounts struct {
	TotalEntries int64
	TotalDone    int64
	FutureTotal  int64
	FutureOpen   int64
	FutureDone   int64
	MonthlyTotal int64
	MonthlyOpen  int64
	MonthlyDone  int64
	WeeklyTotal  int64
	WeeklyOpen   int64
	WeeklyDone   int64
	DueForReview int64
}

// monthCount is one (month, status) bucket of the future log.
type monthCount struct {
	Month  string
	Status string
	N      int64
}

// typeStatusCount is one (type, inspiration, status) bucket, raw from the
// database — inspiration hasn't been folded into "idea" yet, and every
// status (not just open/done) is present, so this is reshaped before it
// reaches the API. See tabOf in the frontend's lib/entryFilters.ts for the
// same task/event/note/idea split this mirrors.
type typeStatusCount struct {
	Type        string
	Inspiration bool
	Status      string
	N           int64
}

// Index gathers everything the INDEX page needs.
//
// Every figure on the page used to be its own COUNT — 31 sequential round
// trips to Postgres for a single page load, which is brutal when the database
// is a managed instance a region away. It is now four queries: one
// conditional-aggregation pass for all the scalar counts, one grouped pass for
// the six future months, plus collections and recent entries. Those four are
// independent, so they run concurrently and the endpoint costs roughly one
// round trip of latency instead of thirty-one.
func (s *OverviewService) Index(userID uint) (*IndexView, error) {
	now := clock.Now()
	month := now.Format("2006-01")
	weekStart, weekEnd := weekBounds(now)

	// Anchor to the 1st before stepping months: AddDate on a day-31 date
	// overflows into the following month (Sep 31 -> Oct 1), which would both
	// skip a month and emit the next one twice.
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	months := make([]string, 0, futureMonthsAhead)
	for i := 0; i < futureMonthsAhead; i++ {
		months = append(months, monthStart.AddDate(0, i, 0).Format("2006-01"))
	}

	view := &IndexView{
		Today:     now.Format("2006-01-02"),
		Month:     month,
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
	}

	var (
		counts      indexCounts
		monthRows   []monthCount
		collections []models.Collection
		recent      []models.Entry
		typeRows    []typeStatusCount
		errs        [5]error
		wg          sync.WaitGroup
	)

	run := func(i int, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = fn()
		}()
	}

	// Each goroutine builds its query from a fresh Scoped() so no two of them
	// ever share a statement being mutated.
	run(0, func() error {
		const sel = `
			COUNT(*) AS total_entries,
			COUNT(*) FILTER (WHERE status = ?) AS total_done,
			COUNT(*) FILTER (WHERE log_kind = ?) AS future_total,
			COUNT(*) FILTER (WHERE log_kind = ? AND status = ?) AS future_open,
			COUNT(*) FILTER (WHERE log_kind = ? AND status = ?) AS future_done,
			COUNT(*) FILTER (WHERE log_kind = ? AND month = ?) AS monthly_total,
			COUNT(*) FILTER (WHERE log_kind = ? AND month = ? AND status = ?) AS monthly_open,
			COUNT(*) FILTER (WHERE log_kind = ? AND month = ? AND status = ?) AS monthly_done,
			COUNT(*) FILTER (WHERE log_kind = ? AND date >= ? AND date <= ?) AS weekly_total,
			COUNT(*) FILTER (WHERE log_kind = ? AND date >= ? AND date <= ? AND status = ?) AS weekly_open,
			COUNT(*) FILTER (WHERE log_kind = ? AND date >= ? AND date <= ? AND status = ?) AS weekly_done,
			COUNT(*) FILTER (
				WHERE status = ? AND (
					(log_kind = ? AND date < ?) OR
					(log_kind = ? AND month < ?) OR
					(log_kind = ? AND month <= ?)
				)
			) AS due_for_review`
		return s.entries.Scoped(userID).Select(sel,
			models.StatusDone,
			models.LogFuture,
			models.LogFuture, models.StatusOpen,
			models.LogFuture, models.StatusDone,
			models.LogMonthly, month,
			models.LogMonthly, month, models.StatusOpen,
			models.LogMonthly, month, models.StatusDone,
			models.LogWeekly, weekStart, weekEnd,
			models.LogWeekly, weekStart, weekEnd, models.StatusOpen,
			models.LogWeekly, weekStart, weekEnd, models.StatusDone,
			models.StatusOpen,
			models.LogWeekly, now.Format("2006-01-02"),
			models.LogMonthly, month,
			models.LogFuture, month,
		).Scan(&counts).Error
	})

	run(1, func() error {
		return s.entries.Scoped(userID).
			Select("month, status, COUNT(*) AS n").
			Where("log_kind = ? AND month IN ?", models.LogFuture, months).
			Group("month, status").
			Scan(&monthRows).Error
	})

	run(2, func() error {
		var err error
		collections, err = s.collections.List(userID)
		return err
	})

	run(3, func() error {
		var err error
		monthEnd := monthStart.AddDate(0, 1, -1).Format("2006-01-02")
		recent, err = s.entries.RecentForUser(userID, 8, month, monthStart.Format("2006-01-02"), monthEnd)
		return err
	})

	run(4, func() error {
		return s.entries.Scoped(userID).
			Select("type, inspiration, status, COUNT(*) AS n").
			Group("type, inspiration, status").
			Scan(&typeRows).Error
	})

	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	view.Logs = []LogSummary{
		{Key: "future", Label: "Future Log", Total: counts.FutureTotal, Open: counts.FutureOpen, Done: counts.FutureDone},
		{Key: "monthly", Label: "Monthly Log", Total: counts.MonthlyTotal, Open: counts.MonthlyOpen, Done: counts.MonthlyDone},
		{Key: "weekly", Label: "Weekly Log", Total: counts.WeeklyTotal, Open: counts.WeeklyOpen, Done: counts.WeeklyDone},
	}

	byMonth := make(map[string]*MonthSummary, len(months))
	for _, row := range monthRows {
		m := byMonth[row.Month]
		if m == nil {
			m = &MonthSummary{Month: row.Month}
			byMonth[row.Month] = m
		}
		m.Total += row.N
		switch models.EntryStatus(row.Status) {
		case models.StatusOpen:
			m.Open += row.N
		case models.StatusDone:
			m.Done += row.N
		}
	}

	view.FutureMonths = make([]MonthSummary, 0, len(months))
	for _, m := range months {
		if summary := byMonth[m]; summary != nil {
			view.FutureMonths = append(view.FutureMonths, *summary)
			continue
		}
		view.FutureMonths = append(view.FutureMonths, MonthSummary{Month: m})
	}

	view.Collections = collections
	view.Recent = recent
	view.Totals.Entries = counts.TotalEntries
	view.Totals.Done = counts.TotalDone
	view.DueForReview = counts.DueForReview
	view.TypeBreakdown = bucketTypeBreakdown(typeRows)

	return view, nil
}

// bucketTypeBreakdown folds the raw (type, inspiration, status) rows into the
// four tabs the app's own entry lists use — an inspiration-flagged row is an
// "idea" regardless of its underlying type, exactly like tabOf() on the
// frontend — and keeps only open/done, the two statuses that UI distinguishes
// with a status filter of their own. Always returns all four tabs, in a
// fixed order, even at zero, so the frontend never has to guess which exist.
func bucketTypeBreakdown(rows []typeStatusCount) []TypeCount {
	tabs := []string{"task", "event", "note", "idea"}
	byTab := make(map[string]*TypeCount, len(tabs))
	for _, tab := range tabs {
		byTab[tab] = &TypeCount{Tab: tab}
	}

	for _, row := range rows {
		tab := row.Type
		if row.Inspiration {
			tab = "idea"
		}
		bucket, ok := byTab[tab]
		if !ok {
			continue // an entry type the UI doesn't have a tab for
		}
		switch models.EntryStatus(row.Status) {
		case models.StatusOpen:
			bucket.Open += row.N
		case models.StatusDone:
			bucket.Done += row.N
		}
	}

	breakdown := make([]TypeCount, len(tabs))
	for i, tab := range tabs {
		breakdown[i] = *byTab[tab]
	}
	return breakdown
}

// Stats returns completion figures for one month.
func (s *OverviewService) Stats(userID uint, month string) (*StatsView, error) {
	if month == "" {
		month = clock.Now().Format("2006-01")
	}
	start := month + "-01"
	t, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, ErrInvalidMonth
	}
	end := t.AddDate(0, 1, -1).Format("2006-01-02")

	statuses := []models.EntryStatus{models.StatusOpen, models.StatusDone, models.StatusMigrated, models.StatusScheduled, models.StatusCancelled}
	types := []models.EntryType{models.TypeTask, models.TypeEvent, models.TypeNote}

	// One grouped pass instead of a COUNT per status and per type: statuses
	// and types are independent breakdowns of the same rows, so grouping by
	// both and summing each dimension separately in Go gets every figure from
	// a single round trip.
	var rows []struct {
		Status string
		Type   string
		N      int64
	}
	if err := s.entries.Scoped(userID).
		Select("status, type, COUNT(*) AS n").
		Where("(month = ? OR (date >= ? AND date <= ?))", month, start, end).
		Group("status, type").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	byStatus := make(map[string]int64, len(statuses))
	for _, st := range statuses {
		byStatus[string(st)] = 0
	}
	byType := make(map[string]int64, len(types))
	for _, ty := range types {
		byType[string(ty)] = 0
	}

	var total int64
	for _, row := range rows {
		total += row.N
		if _, ok := byStatus[row.Status]; ok {
			byStatus[row.Status] += row.N
		}
		if _, ok := byType[row.Type]; ok {
			byType[row.Type] += row.N
		}
	}

	rate := 0.0
	if total > 0 {
		rate = float64(byStatus[string(models.StatusDone)]) / float64(total) * 100
	}

	return &StatsView{Month: month, Total: total, ByStatus: byStatus, ByType: byType, CompletionRate: rate}, nil
}

// weekBounds returns the Monday and Sunday around t as YYYY-MM-DD.
func weekBounds(t time.Time) (string, string) {
	offset := (int(t.Weekday()) + 6) % 7 // Monday = 0
	start := t.AddDate(0, 0, -offset)
	return start.Format("2006-01-02"), start.AddDate(0, 0, 6).Format("2006-01-02")
}
