package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"time"
	_ "time/tzdata" // office timezone must resolve even on hosts without zoneinfo (e.g. Windows)
)

// SchemaVersion is the only document version this build reads or writes.
const SchemaVersion = 1

var (
	ErrNotFound      = errors.New("data file not found")
	ErrCorrupt       = errors.New("data file is malformed")
	ErrSchemaVersion = errors.New("unexpected schema version")
	ErrInvalid       = errors.New("data failed validation")
	ErrLocked        = errors.New("data file is locked by another deskbook instance")
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Document is the complete persisted state.
type Document struct {
	SchemaVersion int        `json:"schemaVersion"`
	Revision      int64      `json:"revision"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	Settings      Settings   `json:"settings"`
	Desks         []Desk     `json:"desks"`
	Employees     []Employee `json:"employees"`
	Bookings      []Booking  `json:"bookings"`
}

type Settings struct {
	OfficeName      string `json:"officeName"`
	FloorName       string `json:"floorName"`
	Timezone        string `json:"timezone"`
	WindowDays      int    `json:"windowDays"`
	OpenHour        int    `json:"openHour"`
	CloseHour       int    `json:"closeHour"`
	BackupRetention int    `json:"backupRetention"`
}

type Desk struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Zone     string  `json:"zone"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Rotation int     `json:"rotation"`
	Active   bool    `json:"active"`
}

type Employee struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	Initials     string    `json:"initials"`
	Role         string    `json:"role"` // "admin" | "employee"
	PasswordHash string    `json:"passwordHash"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Booking struct {
	ID          string     `json:"id"`
	DeskID      string     `json:"deskId"`
	EmployeeID  string     `json:"employeeId"`
	Date        string     `json:"date"` // office-local YYYY-MM-DD
	StartHour   int        `json:"startHour"`
	EndHour     int        `json:"endHour"`
	Status      string     `json:"status"` // "active" | "cancelled"
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
	CancelledAt *time.Time `json:"cancelledAt,omitempty"`
	CancelledBy string     `json:"cancelledBy,omitempty"`
}

// Clone returns a deep copy (time pointers are never mutated in place, so sharing them is safe).
func (d *Document) Clone() *Document {
	c := *d
	c.Desks = slices.Clone(d.Desks)
	c.Employees = slices.Clone(d.Employees)
	c.Bookings = slices.Clone(d.Bookings)
	return &c
}

func (d *Document) Location() *time.Location {
	loc, err := time.LoadLocation(d.Settings.Timezone)
	if err != nil {
		return time.UTC // unreachable for validated documents
	}
	return loc
}

func (d *Document) DeskByID(id string) (int, bool) {
	i := slices.IndexFunc(d.Desks, func(x Desk) bool { return x.ID == id })
	return i, i >= 0
}

func (d *Document) EmployeeByID(id string) (int, bool) {
	i := slices.IndexFunc(d.Employees, func(x Employee) bool { return x.ID == id })
	return i, i >= 0
}

func (d *Document) BookingByID(id string) (int, bool) {
	i := slices.IndexFunc(d.Bookings, func(x Booking) bool { return x.ID == id })
	return i, i >= 0
}

// Decode strictly parses and validates a document. Unknown fields, trailing
// data, a missing or different schemaVersion, and integrity violations are all rejected.
func Decode(data []byte) (*Document, error) {
	var probe struct {
		SchemaVersion *int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if probe.SchemaVersion == nil {
		return nil, fmt.Errorf("%w: schemaVersion missing", ErrCorrupt)
	}
	if *probe.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: got %d, this build supports %d", ErrSchemaVersion, *probe.SchemaVersion, SchemaVersion)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing data after document", ErrCorrupt)
	}
	if err := Validate(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func Encode(doc *Document) ([]byte, error) {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Validate checks document-level integrity. It is applied on load and before every write.
func Validate(d *Document) error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: %d", ErrSchemaVersion, d.SchemaVersion)
	}
	s := d.Settings
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "" || s.Timezone == "Local" {
		return bad("unknown timezone %q", s.Timezone)
	}
	if s.WindowDays < 1 || s.WindowDays > 366 {
		return bad("windowDays must be 1-366")
	}
	if s.OpenHour < 0 || s.CloseHour > 24 || s.OpenHour >= s.CloseHour {
		return bad("opening hours invalid")
	}
	if s.BackupRetention < 1 || s.BackupRetention > 10000 {
		return bad("backupRetention must be 1-10000")
	}

	desks := map[string]bool{}
	for _, x := range d.Desks {
		if !idRe.MatchString(x.ID) || desks[x.ID] {
			return bad("desk id %q invalid or duplicated", x.ID)
		}
		if x.Rotation%90 != 0 || x.Rotation < 0 || x.Rotation >= 360 {
			return bad("desk %s rotation must be 0/90/180/270", x.ID)
		}
		desks[x.ID] = true
	}
	emps, users := map[string]bool{}, map[string]bool{}
	for _, e := range d.Employees {
		if !idRe.MatchString(e.ID) || emps[e.ID] {
			return bad("employee id %q invalid or duplicated", e.ID)
		}
		if e.Username == "" || users[e.Username] {
			return bad("username %q empty or duplicated", e.Username)
		}
		if e.Role != "admin" && e.Role != "employee" {
			return bad("employee %s has unknown role %q", e.ID, e.Role)
		}
		emps[e.ID], users[e.Username] = true, true
	}
	ids := map[string]bool{}
	type slot struct{ desk, date string }
	active := map[slot][]Booking{}
	for _, b := range d.Bookings {
		if !idRe.MatchString(b.ID) || ids[b.ID] {
			return bad("booking id %q invalid or duplicated", b.ID)
		}
		ids[b.ID] = true
		if !desks[b.DeskID] || !emps[b.EmployeeID] {
			return bad("booking %s references unknown desk or employee", b.ID)
		}
		if _, err := time.Parse("2006-01-02", b.Date); err != nil {
			return bad("booking %s has invalid date", b.ID)
		}
		if b.StartHour < 0 || b.EndHour > 24 || b.EndHour <= b.StartHour {
			return bad("booking %s has invalid hours", b.ID)
		}
		switch b.Status {
		case "active":
			k := slot{b.DeskID, b.Date}
			for _, o := range active[k] {
				if b.StartHour < o.EndHour && o.StartHour < b.EndHour {
					return bad("bookings %s and %s overlap", o.ID, b.ID)
				}
			}
			active[k] = append(active[k], b)
		case "cancelled":
		default:
			return bad("booking %s has unknown status %q", b.ID, b.Status)
		}
	}
	return nil
}
