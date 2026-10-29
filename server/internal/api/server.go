// Package api exposes the HTTP JSON API under /api/.
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"time"

	"deskbook/internal/auth"
	"deskbook/internal/booking"
	"deskbook/internal/store"
)

const (
	cookieName   = "deskbook_session"
	csrfHeader   = "X-CSRF-Token"
	maxBodyBytes = 64 << 10
)

type Server struct {
	Store         store.Store
	Sessions      *auth.Manager
	Limiter       *auth.Limiter
	Now           func() time.Time
	SecureCookies bool // set when served over HTTPS
	Log           *slog.Logger
}

func New(st store.Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		Store:    st,
		Sessions: auth.NewManager(12 * time.Hour),
		Limiter:  auth.NewLimiter(5, 15*time.Minute),
		Now:      time.Now,
		Log:      log,
	}
}

type reqCtx struct {
	actor store.Employee
	token string
	sess  auth.Session
}

type handlerFunc func(w http.ResponseWriter, r *http.Request, c *reqCtx)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.authed(s.logout))
	mux.HandleFunc("GET /api/session", s.sessionStatus)
	mux.HandleFunc("GET /api/config", s.authed(s.config))
	mux.HandleFunc("GET /api/desks", s.authed(s.listDesks))
	mux.HandleFunc("GET /api/colleagues", s.authed(s.colleagues))
	mux.HandleFunc("GET /api/bookings", s.authed(s.listBookings))
	mux.HandleFunc("GET /api/bookings/mine", s.authed(s.myBookings))
	mux.HandleFunc("POST /api/bookings", s.authed(s.createBooking))
	mux.HandleFunc("PATCH /api/bookings/{id}", s.authed(s.changeBooking))
	mux.HandleFunc("POST /api/bookings/{id}/cancel", s.authed(s.cancelBooking))

	mux.HandleFunc("GET /api/admin/employees", s.adminOnly(s.listEmployees))
	mux.HandleFunc("POST /api/admin/employees", s.adminOnly(s.createEmployee))
	mux.HandleFunc("PATCH /api/admin/employees/{id}", s.adminOnly(s.updateEmployee))
	mux.HandleFunc("POST /api/admin/desks", s.adminOnly(s.createDesk))
	mux.HandleFunc("PATCH /api/admin/desks/{id}", s.adminOnly(s.updateDesk))
	mux.HandleFunc("DELETE /api/admin/desks/{id}", s.adminOnly(s.deleteDesk))
	mux.HandleFunc("PUT /api/admin/settings", s.adminOnly(s.updateSettings))
	mux.HandleFunc("GET /api/admin/backups", s.adminOnly(s.listBackups))
	mux.HandleFunc("POST /api/admin/backups", s.adminOnly(s.createBackup))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not_found", "", "no such endpoint")
	})
	return mux
}

// authed resolves the session cookie to an active employee loaded from the
// current data (so role changes and deactivation apply immediately) and
// enforces CSRF + same-origin on every state-changing method.
func (s *Server) authed(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie(cookieName)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthenticated", "", "sign in required")
			return
		}
		sess, ok := s.Sessions.Get(ck.Value)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthenticated", "", "session expired, sign in again")
			return
		}
		doc := s.Store.Snapshot()
		i, ok := doc.EmployeeByID(sess.EmployeeID)
		if !ok || !doc.Employees[i].Active {
			s.Sessions.Delete(ck.Value)
			writeErr(w, http.StatusUnauthorized, "unauthenticated", "", "account disabled")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !sameOrigin(r) || !auth.CSRFMatches(sess, r.Header.Get(csrfHeader)) {
				writeErr(w, http.StatusForbidden, "csrf", "", "missing or invalid CSRF token")
				return
			}
		}
		h(w, r, &reqCtx{actor: doc.Employees[i], token: ck.Value, sess: sess})
	}
}

func (s *Server) adminOnly(h handlerFunc) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		if c.actor.Role != "admin" {
			writeErr(w, http.StatusForbidden, booking.CodeForbidden, "", "administrator access required")
			return
		}
		h(w, r, c)
	})
}

// sameOrigin rejects cross-site browser requests. Browsers always send Origin
// on cross-origin POST/PATCH/PUT/DELETE; absent Origin means non-browser or same-origin.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// decode accepts only application/json, bounded in size, with no unknown
// fields and no trailing data. Unknown fields matter: a client cannot smuggle
// "employeeId" or "role" into a request.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return &booking.Error{Code: booking.CodeInvalid, Message: "Content-Type must be application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &booking.Error{Code: booking.CodeInvalid, Message: "malformed JSON: " + err.Error()}
	}
	if _, err := dec.Token(); err != io.EOF {
		return &booking.Error{Code: booking.CodeInvalid, Message: "malformed JSON: trailing data"}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, field, msg string) {
	writeJSON(w, status, map[string]any{"error": booking.Error{Code: code, Field: field, Message: msg}})
}

// fail maps rule errors to 4xx and everything else (storage) to 500. A storage
// failure is never reported as success: the store commits only after the
// atomic rename succeeded.
func (s *Server) fail(w http.ResponseWriter, err error) {
	var be *booking.Error
	if errors.As(err, &be) {
		status := map[string]int{
			booking.CodeInvalid: 400, booking.CodePolicy: 422, booking.CodeConflict: 409,
			booking.CodeForbidden: 403, booking.CodeNotFound: 404, booking.CodeState: 409,
		}[be.Code]
		if status == 0 {
			status = 400
		}
		writeJSON(w, status, map[string]any{"error": be})
		return
	}
	if errors.Is(err, store.ErrInvalid) {
		writeErr(w, http.StatusUnprocessableEntity, booking.CodePolicy, "", err.Error())
		return
	}
	s.Log.Error("storage failure", "err", err)
	writeErr(w, http.StatusInternalServerError, "storage", "",
		"The change could not be saved. Nothing was booked or changed; please try again.")
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revision": s.Store.Snapshot().Revision})
}
