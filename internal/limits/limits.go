// Package limits centralizes the per-account caps that keep one account
// (malicious or just enthusiastic) from growing the database without
// bound. See the README's "Quotas & rate limiting" section for the
// reasoning behind the specific numbers.
package limits

const (
	MaxEntriesPerUser     = 3000
	MaxCollectionsPerUser = 40

	MaxEntryContentLength = 1000
	MaxEntryNotesLength   = 3000

	MaxCollectionTitleLength       = 120
	MaxCollectionDescriptionLength = 500
)
