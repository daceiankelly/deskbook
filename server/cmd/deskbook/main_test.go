package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deskbook/internal/auth"
	"deskbook/internal/store"
)

func TestStaticHandlerNeverExposesData(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>app</html>"), 0o600)
	os.MkdirAll(filepath.Join(root, "assets"), 0o750)
	os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("js"), 0o600)
	os.WriteFile(filepath.Join(root, ".env"), []byte("secret"), 0o600)
	h := staticHandler(root)

	get := func(p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		return rec
	}
	if r := get("/assets/app.js"); r.Code != 200 || r.Body.String() != "js" {
		t.Fatalf("asset: %d", r.Code)
	}
	if r := get("/bookings"); r.Code != 200 || !strings.Contains(r.Body.String(), "app") {
		t.Fatalf("SPA fallback: %d", r.Code)
	}
	for _, p := range []string{"/.env", "/data/deskbook.json", "/../data/deskbook.json", "/deskbook.json"} {
		if r := get(p); r.Code != 404 {
			t.Errorf("%s: want 404 got %d", p, r.Code)
		}
	}
}

func TestSeedIsValid(t *testing.T) {
	auth.Iterations = 1000
	doc, err := seedDocument("a-long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Validate(doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Desks) != 34 {
		t.Fatalf("want 34 desks, got %d", len(doc.Desks))
	}
}

func TestPurgePastBookingsOnStart(t *testing.T) {
	auth.Iterations = 1000
	path := filepath.Join(t.TempDir(), "deskbook.json")
	st, err := store.Open(path, store.Options{Seed: func() (*store.Document, error) {
		d, err := seedDocument("a-long-enough-password")
		if err != nil {
			return nil, err
		}
		for i, date := range []string{"2026-10-26", "2026-10-27", "2026-10-28", "2026-11-30"} {
			d.Bookings = append(d.Bookings, store.Booking{ID: "b" + date, DeskID: d.Desks[i].ID,
				EmployeeID: "e-admin", Date: date, StartHour: 9, EndHour: 10, Status: "active"})
		}
		d.Bookings[1].Status = "cancelled"
		return d, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) // 13:00 in Amsterdam, after today's 09-10 booking ended
	rev := st.Snapshot().Revision

	n, err := purgePastBookings(st, now)
	if err != nil || n != 2 {
		t.Fatalf("want 2 removed, got %d (%v)", n, err)
	}
	var left []string
	for _, b := range st.Snapshot().Bookings {
		left = append(left, b.Date)
	}
	if strings.Join(left, ",") != "2026-10-28,2026-11-30" {
		t.Fatalf("kept %v; today and future must stay", left)
	}
	disk, err := store.VerifyFile(path)
	if err != nil || len(disk.Bookings) != 2 || disk.Revision != rev+1 {
		t.Fatalf("purge not persisted: %v %+v", err, disk)
	}
	// Second start on the same day: nothing to remove, file not rewritten.
	if n, err := purgePastBookings(st, now); err != nil || n != 0 || st.Snapshot().Revision != rev+1 {
		t.Fatalf("second purge: n=%d err=%v rev=%d", n, err, st.Snapshot().Revision)
	}
}

func TestSecurityHeaders(t *testing.T) {
	csp := func(allow bool) string {
		rec := httptest.NewRecorder()
		securityHeaders(http.NotFoundHandler(), allow).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		return rec.Header().Get("Content-Security-Policy")
	}
	def := csp(false)
	if !strings.Contains(def, "connect-src 'self'") || !strings.Contains(def, "'wasm-unsafe-eval'") || strings.Contains(def, "'unsafe-eval'") {
		t.Fatalf("default csp must be strict: %s", def)
	}
	if !strings.Contains(csp(true), "'unsafe-eval'") {
		t.Fatal("-allow-unsafe-eval did not relax the csp")
	}
}
