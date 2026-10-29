package api

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"deskbook/internal/auth"
	"deskbook/internal/booking"
	"deskbook/internal/store"
)

var (
	usernameRe = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)
	deskIDRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

func invalid(field, format string, a ...any) error {
	return &booking.Error{Code: booking.CodeInvalid, Field: field, Message: fmt.Sprintf(format, a...)}
}

func initialsOf(name string) string {
	var b strings.Builder
	for _, f := range strings.Fields(name) {
		r, _ := utf8.DecodeRuneInString(f)
		b.WriteRune(r)
		if b.Len() >= 2 {
			break
		}
	}
	return strings.ToUpper(b.String())
}

// ---- employees ----

func (s *Server) listEmployees(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	doc := s.Store.Snapshot()
	out := make([]employeeView, 0, len(doc.Employees))
	for _, e := range doc.Employees {
		out = append(out, viewEmployee(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"employees": out})
}

func (s *Server) createEmployee(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var body struct {
		Username string `json:"username"`
		Name     string `json:"name"`
		Initials string `json:"initials"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, err)
		return
	}
	body.Username = strings.ToLower(strings.TrimSpace(body.Username))
	body.Name = strings.TrimSpace(body.Name)
	switch {
	case !usernameRe.MatchString(body.Username):
		s.fail(w, invalid("username", "username must be 3-32 chars: a-z 0-9 . _ -"))
		return
	case body.Name == "" || utf8.RuneCountInString(body.Name) > 100:
		s.fail(w, invalid("name", "name is required (max 100 chars)"))
		return
	case body.Role != "admin" && body.Role != "employee":
		s.fail(w, invalid("role", "role must be admin or employee"))
		return
	case utf8.RuneCountInString(body.Initials) > 3:
		s.fail(w, invalid("initials", "initials max 3 chars"))
		return
	}
	hash, err := auth.HashPassword(body.Password) // slow: done outside the store lock
	if err != nil {
		s.fail(w, invalid("password", "%s", err.Error()))
		return
	}
	if body.Initials == "" {
		body.Initials = initialsOf(body.Name)
	}
	emp := store.Employee{ID: newID("e"), Username: body.Username, Name: body.Name,
		Initials: strings.ToUpper(body.Initials), Role: body.Role, PasswordHash: hash, Active: true,
		CreatedAt: s.Now().UTC()}
	_, err = s.Store.Update(func(doc *store.Document) error {
		if slices.ContainsFunc(doc.Employees, func(e store.Employee) bool { return e.Username == emp.Username }) {
			return &booking.Error{Code: booking.CodeConflict, Field: "username", Message: "username already exists"}
		}
		doc.Employees = append(doc.Employees, emp)
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"employee": viewEmployee(emp)})
}

func (s *Server) updateEmployee(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var body struct {
		Name     *string `json:"name"`
		Initials *string `json:"initials"`
		Role     *string `json:"role"`
		Active   *bool   `json:"active"`
		Password *string `json:"password"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, err)
		return
	}
	id := r.PathValue("id")
	var hash string
	if body.Password != nil {
		h, err := auth.HashPassword(*body.Password)
		if err != nil {
			s.fail(w, invalid("password", "%s", err.Error()))
			return
		}
		hash = h
	}
	var updated store.Employee
	_, err := s.Store.Update(func(doc *store.Document) error {
		i, ok := doc.EmployeeByID(id)
		if !ok {
			return &booking.Error{Code: booking.CodeNotFound, Message: "employee not found"}
		}
		e := doc.Employees[i]
		if body.Name != nil {
			n := strings.TrimSpace(*body.Name)
			if n == "" || utf8.RuneCountInString(n) > 100 {
				return invalid("name", "name is required (max 100 chars)")
			}
			e.Name = n
		}
		if body.Initials != nil {
			if utf8.RuneCountInString(*body.Initials) > 3 {
				return invalid("initials", "initials max 3 chars")
			}
			e.Initials = strings.ToUpper(*body.Initials)
		}
		if body.Role != nil {
			if *body.Role != "admin" && *body.Role != "employee" {
				return invalid("role", "role must be admin or employee")
			}
			e.Role = *body.Role
		}
		if body.Active != nil {
			e.Active = *body.Active
		}
		if id == c.actor.ID && (e.Role != "admin" || !e.Active) {
			return &booking.Error{Code: booking.CodePolicy, Message: "you cannot remove your own administrator access"}
		}
		if hash != "" {
			e.PasswordHash = hash
		}
		doc.Employees[i] = e
		if !e.Active { // free the desks of someone who has left
			now := s.Now()
			t := now.UTC()
			for j, b := range doc.Bookings {
				if b.EmployeeID == id && b.Status == "active" && !booking.HasEnded(b, doc.Settings, now) {
					b.Status, b.CancelledAt, b.CancelledBy = "cancelled", &t, c.actor.ID
					doc.Bookings[j] = b
				}
			}
		}
		updated = e
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	if !updated.Active || hash != "" {
		s.Sessions.DeleteForEmployee(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"employee": viewEmployee(updated)})
}

// ---- desks / floor plan ----

type deskBody struct {
	ID       *string  `json:"id"`
	Label    *string  `json:"label"`
	Zone     *string  `json:"zone"`
	X        *float64 `json:"x"`
	Y        *float64 `json:"y"`
	Rotation *int     `json:"rotation"`
	Active   *bool    `json:"active"`
}

func applyDesk(d *store.Desk, b deskBody) error {
	if b.Label != nil {
		l := strings.TrimSpace(*b.Label)
		if l == "" || utf8.RuneCountInString(l) > 20 {
			return invalid("label", "label is required (max 20 chars)")
		}
		d.Label = l
	}
	if b.Zone != nil {
		if utf8.RuneCountInString(*b.Zone) > 40 {
			return invalid("zone", "zone max 40 chars")
		}
		d.Zone = *b.Zone
	}
	if b.X != nil {
		if *b.X < 0 || *b.X > 2000 {
			return invalid("x", "x must be 0-2000")
		}
		d.X = *b.X
	}
	if b.Y != nil {
		if *b.Y < 0 || *b.Y > 2000 {
			return invalid("y", "y must be 0-2000")
		}
		d.Y = *b.Y
	}
	if b.Rotation != nil {
		if *b.Rotation%90 != 0 || *b.Rotation < 0 || *b.Rotation >= 360 {
			return invalid("rotation", "rotation must be 0, 90, 180 or 270")
		}
		d.Rotation = *b.Rotation
	}
	if b.Active != nil {
		d.Active = *b.Active
	}
	return nil
}

func (s *Server) createDesk(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var body deskBody
	if err := decode(w, r, &body); err != nil {
		s.fail(w, err)
		return
	}
	if body.Label == nil || body.X == nil || body.Y == nil {
		s.fail(w, invalid("label", "label, x and y are required"))
		return
	}
	var created store.Desk
	_, err := s.Store.Update(func(doc *store.Document) error {
		d := store.Desk{Active: true}
		if body.ID != nil && *body.ID != "" {
			if !deskIDRe.MatchString(*body.ID) {
				return invalid("id", "id may contain A-Z a-z 0-9 _ - only")
			}
			if _, exists := doc.DeskByID(*body.ID); exists {
				return &booking.Error{Code: booking.CodeConflict, Field: "id", Message: "desk id already exists"}
			}
			d.ID = *body.ID
		} else {
			for n := len(doc.Desks) + 1; ; n++ {
				id := fmt.Sprintf("D-%02d", n)
				if _, exists := doc.DeskByID(id); !exists {
					d.ID = id
					break
				}
			}
		}
		if err := applyDesk(&d, body); err != nil {
			return err
		}
		doc.Desks = append(doc.Desks, d)
		created = d
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"desk": created})
}

func (s *Server) updateDesk(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var body deskBody
	if err := decode(w, r, &body); err != nil {
		s.fail(w, err)
		return
	}
	if body.ID != nil {
		s.fail(w, invalid("id", "desk id cannot be changed"))
		return
	}
	id := r.PathValue("id")
	var updated store.Desk
	_, err := s.Store.Update(func(doc *store.Document) error {
		i, ok := doc.DeskByID(id)
		if !ok {
			return &booking.Error{Code: booking.CodeNotFound, Message: "desk not found"}
		}
		d := doc.Desks[i]
		if err := applyDesk(&d, body); err != nil {
			return err
		}
		doc.Desks[i] = d
		updated = d
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"desk": updated})
}

func (s *Server) deleteDesk(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := r.PathValue("id")
	_, err := s.Store.Update(func(doc *store.Document) error {
		i, ok := doc.DeskByID(id)
		if !ok {
			return &booking.Error{Code: booking.CodeNotFound, Message: "desk not found"}
		}
		if slices.ContainsFunc(doc.Bookings, func(b store.Booking) bool { return b.DeskID == id }) {
			return &booking.Error{Code: booking.CodeConflict,
				Message: "desk has booking history; deactivate it instead of deleting"}
		}
		doc.Desks = slices.Delete(doc.Desks, i, i+1)
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- settings & backups ----

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var body store.Settings
	if err := decode(w, r, &body); err != nil {
		s.fail(w, err)
		return
	}
	if body.Timezone == "Local" {
		s.fail(w, invalid("timezone", "use an IANA zone such as Europe/Amsterdam"))
		return
	}
	doc, err := s.Store.Update(func(doc *store.Document) error {
		doc.Settings = body
		return nil // store.Validate rejects bad values -> 422
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": doc.Settings})
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	list, err := s.Store.ListBackups()
	if err != nil {
		s.fail(w, err)
		return
	}
	if list == nil {
		list = []store.BackupInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": list})
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	name, err := s.Store.Backup()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"name": name})
}
