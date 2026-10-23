// Package clock defines court-calendar time for NithiDesk.
//
// Court dates are local calendar dates in Sri Lanka with no time zone
// (logical model D12). They are carried as time.Time at midnight UTC so that
// a database `date` round-trips without shifting, and "today" is always the
// date in Asia/Colombo, never the server's local date.
package clock

import "time"

// Colombo is the Sri Lankan time zone. Sri Lanka has no daylight saving, so
// the fixed +05:30 fallback is exact if the tz database is unavailable.
var Colombo = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		return time.FixedZone("Asia/Colombo", 5*3600+1800)
	}
	return loc
}()

// Now returns the current instant; tests replace it.
var Now = time.Now

// Today is the current court date in Sri Lanka, as a date value.
func Today() time.Time { return DateOf(Now()) }

// DateOf returns the Sri Lankan calendar date of instant t.
func DateOf(t time.Time) time.Time {
	l := t.In(Colombo)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// ParseDate parses YYYY-MM-DD into a date value.
func ParseDate(s string) (time.Time, error) { return time.Parse(time.DateOnly, s) }

// FormatDate formats a date value as YYYY-MM-DD.
func FormatDate(t time.Time) string { return t.Format(time.DateOnly) }
