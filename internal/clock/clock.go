// Package clock is the single place this app decides what "today" and
// "this month" mean.
//
// Every host this has run on so far (a laptop, a container on Render)
// defaults its OS clock to UTC, while every user of this app is in Thailand.
// Anything that used plain time.Now() for a calendar concept — the current
// month on the Index page, what counts as "arrived" from the Future Log,
// today's date for the Review ritual — would agree with the user's own
// clock for most of the day and silently disagree with it for the seven
// hours (00:00–06:59 ICT) that fall on the *previous* UTC day. That's
// exactly the "it's September 1st, why does Monthly Log still say August"
// report this fixes: at 00:00–06:59 ICT on the 1st, the UTC clock still
// reads the 31st.
package clock

import (
	"log"
	"time"
)

// Location is fixed to Thailand. The app has exactly one timezone of users
// today; if that changes, this is the one place a per-user timezone would
// plug in instead.
var Location = mustLoad("Asia/Bangkok")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Falls back to server-local time rather than crashing — a wrong
		// clock for a few calendar edge cases beats the service refusing to
		// start because a container image is missing tzdata.
		log.Printf("clock: could not load %s, falling back to server-local time: %v", name, err)
		return time.Local
	}
	return loc
}

// Now is time.Now, fixed to Location. Use this — not time.Now() — for
// anything that answers "what day/month is it," as opposed to measuring a
// duration (a session's expiry, a rate limiter's window), where the
// timezone cancels out and plain time.Now() remains correct and simpler.
func Now() time.Time {
	return time.Now().In(Location)
}
