// Package timewindow defines the two date ranges used by the loader and the
// notifier to address the same activities consistently:
//
//   - Near:     Monday of the current week … Sunday two weeks later (3 weeks).
//   - LongTerm: the day after Near … the last day of (current month + 2).
//
// Keeping the math here means both packages always agree on which slot belongs
// to which window, and on the boundary between them.
package timewindow

import "time"

// Near returns the [from, till] window covering the current ISO week (Monday)
// and the two following weeks — three weeks total, inclusive on both ends.
func Near(now time.Time) (from, till time.Time) {
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// time.Weekday(): Sunday=0..Saturday=6. Convert to Monday-based weekday: Mon=0..Sun=6.
	weekday := (int(startOfDay.Weekday()) + 6) % 7
	from = startOfDay.AddDate(0, 0, -weekday)
	till = from.AddDate(0, 0, 21-1) // Sunday of week+2 (inclusive)
	return
}

// LongTerm returns the [from, till] window that starts the day after Near and
// ends on the last day of (current month + 2). If the near range already
// covers the entire long-term horizon, till is before from and the caller
// should skip the load.
func LongTerm(now time.Time) (from, till time.Time) {
	_, nearTill := Near(now)
	from = nearTill.AddDate(0, 0, 1)
	// First day of (current month + 3) − 1 day == last day of (current month + 2).
	till = time.Date(now.Year(), now.Month()+3, 1, 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -1)
	return
}

