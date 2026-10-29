// Package booking holds the server-side booking rules. These mirror the
// Python module in web/src/python/deskbook but are the authoritative check:
// nothing the browser computes is trusted.
package booking

import (
	"fmt"
	"regexp"
	"time"

	"deskbook/internal/store"
)

// Error codes map to HTTP statuses in the API layer.
const (
	CodeInvalid   = "invalid"   // 400 malformed input
	CodePolicy    = "policy"    // 422 well-formed but not allowed
	CodeConflict  = "conflict"  // 409 overlaps another booking
	CodeForbidden = "forbidden" // 403
	CodeNotFound  = "not_found" // 404
	CodeState     = "state"     // 409 e.g. already cancelled
)

type Error struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func fail(code, field, format string, a ...any) *Error {
	return &Error{Code: code, Field: field, Message: fmt.Sprintf(format, a...)}
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Request is the only booking input accepted from clients. The employee is
// always taken from the authenticated session, never from the body.
type Request struct {
	DeskID    string `json:"deskId"`
	Date      string `json:"date"`
	StartHour int    `json:"startHour"`
	EndHour   int    `json:"endHour"`
}

// Overlaps uses half-open intervals so back-to-back bookings are allowed.
func Overlaps(aStart, aEnd, bStart, bEnd int) bool { return aStart < bEnd && bStart < aEnd }

// Clock describes "now" in the office timezone.
type Clock struct {
	Today   time.Time // midnight, as a UTC-based civil date for day arithmetic
	NowHour int
}

func OfficeClock(s store.Settings, now time.Time) Clock {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		loc = time.UTC
	}
	t := now.In(loc)
	return Clock{Today: civil(t.Year(), t.Month(), t.Day()), NowHour: t.Hour()}
}

func civil(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Format("2006-01-02") != s {
		return time.Time{}, fmt.Errorf("date must be YYYY-MM-DD")
	}
	return t, nil
}

// WindowBounds returns the first and last bookable dates (inclusive).
func WindowBounds(s store.Settings, c Clock) (time.Time, time.Time) {
	return c.Today, c.Today.AddDate(0, 0, s.WindowDays-1)
}

// Validate checks a new or changed booking against the latest document.
// ignoreID excludes the booking being changed; employeeID is the owner.
func Validate(doc *store.Document, r Request, employeeID string, now time.Time, ignoreID string) error {
	if !idRe.MatchString(r.DeskID) {
		return fail(CodeInvalid, "deskId", "invalid desk id")
	}
	d, err := ParseDate(r.Date)
	if err != nil {
		return fail(CodeInvalid, "date", "date must be YYYY-MM-DD")
	}
	if r.StartHour < 0 || r.StartHour > 24 || r.EndHour < 0 || r.EndHour > 24 {
		return fail(CodeInvalid, "startHour", "hours must be 0-24")
	}
	if r.EndHour <= r.StartHour {
		return fail(CodeInvalid, "endHour", "end time must be later than start time")
	}

	s := doc.Settings
	clk := OfficeClock(s, now)
	first, last := WindowBounds(s, clk)
	if d.Before(first) {
		return fail(CodePolicy, "date", "date is in the past")
	}
	if d.After(last) {
		return fail(CodePolicy, "date", "date is beyond the %d-day booking window", s.WindowDays)
	}
	if r.StartHour < s.OpenHour || r.EndHour > s.CloseHour {
		return fail(CodePolicy, "startHour", "bookings must be within %02d:00-%02d:00", s.OpenHour, s.CloseHour)
	}
	if d.Equal(clk.Today) && r.StartHour < clk.NowHour {
		return fail(CodePolicy, "startHour", "start time has already passed")
	}

	i, ok := doc.DeskByID(r.DeskID)
	if !ok {
		return fail(CodeNotFound, "deskId", "desk does not exist")
	}
	if !doc.Desks[i].Active {
		return fail(CodePolicy, "deskId", "desk is unavailable")
	}

	for _, b := range doc.Bookings {
		if b.Status != "active" || b.ID == ignoreID || b.Date != r.Date {
			continue
		}
		if !Overlaps(r.StartHour, r.EndHour, b.StartHour, b.EndHour) {
			continue
		}
		if b.DeskID == r.DeskID {
			return fail(CodeConflict, "deskId", "desk already booked %02d:00-%02d:00", b.StartHour, b.EndHour)
		}
		if b.EmployeeID == employeeID {
			return fail(CodeConflict, "startHour", "you already have a booking at that time")
		}
	}
	return nil
}

// HasEnded reports whether the booking's end time is now or in the past (office time).
func HasEnded(b store.Booking, s store.Settings, now time.Time) bool {
	clk := OfficeClock(s, now)
	d, err := ParseDate(b.Date)
	if err != nil {
		return true
	}
	return d.Before(clk.Today) || (d.Equal(clk.Today) && b.EndHour <= clk.NowHour)
}

// HasStarted reports whether the booking's start hour has been reached.
func HasStarted(b store.Booking, s store.Settings, now time.Time) bool {
	clk := OfficeClock(s, now)
	d, err := ParseDate(b.Date)
	if err != nil {
		return true
	}
	return d.Before(clk.Today) || (d.Equal(clk.Today) && b.StartHour <= clk.NowHour)
}

// PurgeBeforeToday removes every booking (active or cancelled) dated before
// the office's current date and returns how many were removed. Today's
// bookings are kept even if they have already ended.
func PurgeBeforeToday(doc *store.Document, now time.Time) int {
	today := OfficeClock(doc.Settings, now).Today
	kept := doc.Bookings[:0]
	for _, b := range doc.Bookings {
		if d, err := ParseDate(b.Date); err == nil && d.Before(today) {
			continue
		}
		kept = append(kept, b)
	}
	removed := len(doc.Bookings) - len(kept)
	doc.Bookings = kept
	return removed
}

// CanModify: owners manage their own bookings, administrators manage all.
func CanModify(b store.Booking, actor store.Employee) bool {
	return actor.Role == "admin" || b.EmployeeID == actor.ID
}
