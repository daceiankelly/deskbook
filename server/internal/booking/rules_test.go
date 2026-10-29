package booking

import (
	"errors"
	"testing"
	"time"

	"deskbook/internal/store"
)

func doc() *store.Document {
	return &store.Document{
		SchemaVersion: store.SchemaVersion,
		Settings: store.Settings{Timezone: "Europe/Amsterdam", WindowDays: 60, OpenHour: 7, CloseHour: 19,
			BackupRetention: 5},
		Desks: []store.Desk{{ID: "D-01", Active: true}, {ID: "D-02", Active: true}, {ID: "D-03", Active: false}},
		Employees: []store.Employee{
			{ID: "u1", Username: "u1", Role: "employee", Active: true},
			{ID: "u2", Username: "u2", Role: "employee", Active: true},
		},
		Bookings: []store.Booking{
			{ID: "b1", DeskID: "D-01", EmployeeID: "u2", Date: "2026-10-29", StartHour: 9, EndHour: 12, Status: "active"},
			{ID: "bx", DeskID: "D-02", EmployeeID: "u2", Date: "2026-10-29", StartHour: 9, EndHour: 12, Status: "cancelled"},
		},
	}
}

// 08:00 Amsterdam (CET, UTC+1) on 2026-10-28.
var now = time.Date(2026, 10, 28, 7, 0, 0, 0, time.UTC)

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if err == nil {
		return ""
	}
	return "other"
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		r    Request
		emp  string
		want string
	}{
		{"ok", Request{"D-02", "2026-10-29", 9, 10}, "u1", ""},
		{"end before start", Request{"D-02", "2026-10-29", 10, 9}, "u1", CodeInvalid},
		{"zero length", Request{"D-02", "2026-10-29", 10, 10}, "u1", CodeInvalid},
		{"hour out of range", Request{"D-02", "2026-10-29", -1, 25}, "u1", CodeInvalid},
		{"bad date", Request{"D-02", "2026-02-30", 9, 10}, "u1", CodeInvalid},
		{"bad date format", Request{"D-02", "29-10-2026", 9, 10}, "u1", CodeInvalid},
		{"bad desk id", Request{"../x", "2026-10-29", 9, 10}, "u1", CodeInvalid},
		{"overlap", Request{"D-01", "2026-10-29", 11, 13}, "u1", CodeConflict},
		{"contained", Request{"D-01", "2026-10-29", 10, 11}, "u1", CodeConflict},
		{"back to back after", Request{"D-01", "2026-10-29", 12, 13}, "u1", ""},
		{"back to back before", Request{"D-01", "2026-10-29", 7, 9}, "u1", ""},
		{"cancelled ignored", Request{"D-02", "2026-10-29", 9, 12}, "u1", ""},
		{"same employee two desks", Request{"D-02", "2026-10-29", 10, 11}, "u2", CodeConflict},
		{"inactive desk", Request{"D-03", "2026-10-29", 9, 10}, "u1", CodePolicy},
		{"unknown desk", Request{"D-99", "2026-10-29", 9, 10}, "u1", CodeNotFound},
		{"before opening", Request{"D-02", "2026-10-29", 6, 8}, "u1", CodePolicy},
		{"after closing", Request{"D-02", "2026-10-29", 18, 20}, "u1", CodePolicy},
		{"yesterday", Request{"D-02", "2026-10-27", 9, 10}, "u1", CodePolicy},
		{"today earlier hour", Request{"D-02", "2026-10-28", 7, 9}, "u1", CodePolicy},
		{"today current hour", Request{"D-02", "2026-10-28", 8, 9}, "u1", ""},
		{"day 60 (last)", Request{"D-02", "2026-12-26", 9, 10}, "u1", ""},
		{"day 61", Request{"D-02", "2026-12-27", 9, 10}, "u1", CodePolicy},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := code(Validate(doc(), c.r, c.emp, now, "")); got != c.want {
				t.Fatalf("want %q got %q", c.want, got)
			}
		})
	}
}

func TestChangeIgnoresItself(t *testing.T) {
	if err := Validate(doc(), Request{"D-01", "2026-10-29", 10, 13}, "u2", now, "b1"); err != nil {
		t.Fatal(err)
	}
}

// The office date, not the UTC date, decides "today": at 23:30 UTC on 28 Oct
// it is already 29 Oct 00:30 in Amsterdam.
func TestOfficeTimezoneDecidesToday(t *testing.T) {
	late := time.Date(2026, 10, 28, 23, 30, 0, 0, time.UTC)
	if got := code(Validate(doc(), Request{"D-02", "2026-10-28", 9, 10}, "u1", late, "")); got != CodePolicy {
		t.Fatalf("28 Oct should be in the past in office time, got %q", got)
	}
	// Window end moves with the office date too: 29 Oct + 59 = 27 Dec.
	if err := Validate(doc(), Request{"D-02", "2026-12-27", 9, 10}, "u1", late, ""); err != nil {
		t.Fatal(err)
	}
}

// Across the DST change (25 Oct 2026) the window is still counted in calendar days.
func TestWindowAcrossDST(t *testing.T) {
	before := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	if err := Validate(doc(), Request{"D-02", "2026-12-18", 9, 10}, "u1", before, ""); err != nil {
		t.Fatal(err)
	}
	if code(Validate(doc(), Request{"D-02", "2026-12-19", 9, 10}, "u1", before, "")) != CodePolicy {
		t.Fatal("day 61 accepted")
	}
}

func TestPurgeBeforeToday(t *testing.T) {
	d := doc()
	d.Bookings = append(d.Bookings,
		store.Booking{ID: "old", DeskID: "D-01", EmployeeID: "u1", Date: "2026-10-27", StartHour: 9, EndHour: 10, Status: "active"},
		store.Booking{ID: "oldx", DeskID: "D-01", EmployeeID: "u1", Date: "2026-09-01", StartHour: 9, EndHour: 10, Status: "cancelled"},
		store.Booking{ID: "today", DeskID: "D-01", EmployeeID: "u1", Date: "2026-10-28", StartHour: 7, EndHour: 8, Status: "active"},
	)
	if n := PurgeBeforeToday(d, now); n != 2 {
		t.Fatalf("want 2 removed, got %d", n)
	}
	var ids []string
	for _, b := range d.Bookings {
		ids = append(ids, b.ID)
	}
	if len(ids) != 3 || ids[2] != "today" {
		t.Fatalf("kept %v", ids)
	}
	// Office time decides "previous day": 23:30 UTC on 28 Oct is already 29 Oct in Amsterdam.
	if n := PurgeBeforeToday(d, time.Date(2026, 10, 28, 23, 30, 0, 0, time.UTC)); n != 1 {
		t.Fatalf("want today's (28 Oct) booking purged after office midnight, got %d", n)
	}
}

func TestEndedStartedAndOwnership(t *testing.T) {
	d := doc()
	b := d.Bookings[0] // 29 Oct 09-12
	s := d.Settings
	at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour-1, 0, 0, 0, time.UTC) } // CET=UTC+1
	if HasStarted(b, s, at(29, 8)) || !HasStarted(b, s, at(29, 9)) {
		t.Fatal("HasStarted wrong")
	}
	if HasEnded(b, s, at(29, 11)) || !HasEnded(b, s, at(29, 12)) {
		t.Fatal("HasEnded wrong")
	}
	if CanModify(b, store.Employee{ID: "u1", Role: "employee"}) ||
		!CanModify(b, store.Employee{ID: "u2", Role: "employee"}) ||
		!CanModify(b, store.Employee{ID: "u1", Role: "admin"}) {
		t.Fatal("CanModify wrong")
	}
}
