package api

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"deskbook/internal/auth"
	"deskbook/internal/booking"
	"deskbook/internal/store"
)

type employeeView struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
}

func viewEmployee(e store.Employee) employeeView {
	return employeeView{e.ID, e.Username, e.Name, e.Initials, e.Role, e.Active}
}

type bookingView struct {
	store.Booking
	EmployeeName     string `json:"employeeName"`
	EmployeeInitials string `json:"employeeInitials"`
	DeskLabel        string `json:"deskLabel"`
	Mine             bool   `json:"mine"`
}

func viewBooking(doc *store.Document, b store.Booking, actorID string) bookingView {
	v := bookingView{Booking: b, Mine: b.EmployeeID == actorID}
	if i, ok := doc.EmployeeByID(b.EmployeeID); ok {
		v.EmployeeName, v.EmployeeInitials = doc.Employees[i].Name, doc.Employees[i].Initials
	}
	if i, ok := doc.DeskByID(b.DeskID); ok {
		v.DeskLabel = doc.Desks[i].Label
	}
	return v
}

// ---- authentication ----

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "csrf", "", "cross-origin login rejected")
		return
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, err)
		return
	}
	username := strings.ToLower(strings.TrimSpace(body.Username))
	key := username + "|" + clientIP(r)
	if s.Limiter.Blocked(key) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "", "too many failed attempts, try again later")
		return
	}
	doc := s.Store.Snapshot()
	i := slices.IndexFunc(doc.Employees, func(e store.Employee) bool { return e.Username == username && e.Active })
	if i < 0 {
		auth.VerifyDummy(body.Password)
		s.Limiter.Fail(key)
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "", "incorrect username or password")
		return
	}
	emp := doc.Employees[i]
	if !auth.VerifyPassword(body.Password, emp.PasswordHash) {
		s.Limiter.Fail(key)
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "", "incorrect username or password")
		return
	}
	s.Limiter.Reset(key)
	token, sess := s.Sessions.Create(emp.ID)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: s.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"user": viewEmployee(emp), "csrfToken": sess.CSRF})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.Sessions.Delete(c.token)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: s.SecureCookies, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// sessionStatus answers "am I signed in?" with 200 either way, so the app's
// startup check does not log a 401 in the browser console when signed out.
func (s *Server) sessionStatus(w http.ResponseWriter, r *http.Request) {
	signedOut := map[string]any{"user": nil}
	ck, err := r.Cookie(cookieName)
	if err != nil {
		writeJSON(w, http.StatusOK, signedOut)
		return
	}
	sess, ok := s.Sessions.Get(ck.Value)
	if !ok {
		writeJSON(w, http.StatusOK, signedOut)
		return
	}
	doc := s.Store.Snapshot()
	i, ok := doc.EmployeeByID(sess.EmployeeID)
	if !ok || !doc.Employees[i].Active {
		s.Sessions.Delete(ck.Value)
		writeJSON(w, http.StatusOK, signedOut)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": viewEmployee(doc.Employees[i]), "csrfToken": sess.CSRF})
}

// ---- reference data ----

func (s *Server) config(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	doc := s.Store.Snapshot()
	now := s.Now()
	clk := booking.OfficeClock(doc.Settings, now)
	_, last := booking.WindowBounds(doc.Settings, clk)
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":   doc.Settings,
		"today":      clk.Today.Format("2006-01-02"),
		"lastDate":   last.Format("2006-01-02"),
		"nowHour":    clk.NowHour,
		"serverTime": now.UTC(),
		"user":       viewEmployee(c.actor),
	})
}

func (s *Server) listDesks(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, map[string]any{"desks": s.Store.Snapshot().Desks})
}

func (s *Server) colleagues(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	doc := s.Store.Snapshot()
	out := []map[string]string{}
	for _, e := range doc.Employees {
		if e.Active {
			out = append(out, map[string]string{"id": e.ID, "name": e.Name, "initials": e.Initials})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"colleagues": out})
}

// ---- bookings ----

func (s *Server) listBookings(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	doc := s.Store.Snapshot()
	clk := booking.OfficeClock(doc.Settings, s.Now())
	first, last := booking.WindowBounds(doc.Settings, clk)
	from, to := first, last
	if v := r.URL.Query().Get("from"); v != "" {
		d, err := booking.ParseDate(v)
		if err != nil {
			writeErr(w, 400, booking.CodeInvalid, "from", err.Error())
			return
		}
		from = d
	}
	if v := r.URL.Query().Get("to"); v != "" {
		d, err := booking.ParseDate(v)
		if err != nil {
			writeErr(w, 400, booking.CodeInvalid, "to", err.Error())
			return
		}
		to = d
	}
	if to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		writeErr(w, 400, booking.CodeInvalid, "to", "invalid date range")
		return
	}
	out := []bookingView{}
	for _, b := range doc.Bookings {
		d, _ := booking.ParseDate(b.Date)
		if b.Status == "active" && !d.Before(from) && !d.After(to) {
			out = append(out, viewBooking(doc, b, c.actor.ID))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookings": out, "revision": doc.Revision})
}

func (s *Server) myBookings(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	doc := s.Store.Snapshot()
	out := []bookingView{}
	for _, b := range doc.Bookings {
		if b.EmployeeID == c.actor.ID {
			out = append(out, viewBooking(doc, b, c.actor.ID))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookings": out})
}

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var req booking.Request
	if err := decode(w, r, &req); err != nil {
		s.fail(w, err)
		return
	}
	var created store.Booking
	doc, err := s.Store.Update(func(doc *store.Document) error {
		now := s.Now()
		if err := booking.Validate(doc, req, c.actor.ID, now, ""); err != nil {
			return err
		}
		created = store.Booking{
			ID: newID("b"), DeskID: req.DeskID, EmployeeID: c.actor.ID, Date: req.Date,
			StartHour: req.StartHour, EndHour: req.EndHour, Status: "active", CreatedAt: now.UTC(),
		}
		doc.Bookings = append(doc.Bookings, created)
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"booking": viewBooking(doc, created, c.actor.ID)})
}

func (s *Server) changeBooking(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var req booking.Request
	if err := decode(w, r, &req); err != nil {
		s.fail(w, err)
		return
	}
	id := r.PathValue("id")
	var changed store.Booking
	doc, err := s.Store.Update(func(doc *store.Document) error {
		i, ok := doc.BookingByID(id)
		if !ok {
			return &booking.Error{Code: booking.CodeNotFound, Message: "booking not found"}
		}
		b := doc.Bookings[i]
		if !booking.CanModify(b, c.actor) {
			return &booking.Error{Code: booking.CodeForbidden, Message: "you can only change your own bookings"}
		}
		if b.Status != "active" {
			return &booking.Error{Code: booking.CodeState, Message: "booking is cancelled"}
		}
		now := s.Now()
		if booking.HasStarted(b, doc.Settings, now) {
			return &booking.Error{Code: booking.CodePolicy, Message: "booking has already started; cancel it instead"}
		}
		if err := booking.Validate(doc, req, b.EmployeeID, now, b.ID); err != nil {
			return err
		}
		t := now.UTC()
		b.DeskID, b.Date, b.StartHour, b.EndHour, b.UpdatedAt = req.DeskID, req.Date, req.StartHour, req.EndHour, &t
		doc.Bookings[i] = b
		changed = b
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"booking": viewBooking(doc, changed, c.actor.ID)})
}

func (s *Server) cancelBooking(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := r.PathValue("id")
	var cancelled store.Booking
	doc, err := s.Store.Update(func(doc *store.Document) error {
		i, ok := doc.BookingByID(id)
		if !ok {
			return &booking.Error{Code: booking.CodeNotFound, Message: "booking not found"}
		}
		b := doc.Bookings[i]
		if !booking.CanModify(b, c.actor) {
			return &booking.Error{Code: booking.CodeForbidden, Message: "you can only cancel your own bookings"}
		}
		if b.Status != "active" {
			return &booking.Error{Code: booking.CodeState, Message: "booking is already cancelled"}
		}
		now := s.Now()
		if booking.HasEnded(b, doc.Settings, now) {
			return &booking.Error{Code: booking.CodePolicy, Message: "booking has already ended"}
		}
		t := now.UTC()
		b.Status, b.CancelledAt, b.CancelledBy = "cancelled", &t, c.actor.ID
		doc.Bookings[i] = b
		cancelled = b
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"booking": viewBooking(doc, cancelled, c.actor.ID)})
}
