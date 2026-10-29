package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"deskbook/internal/auth"
	"deskbook/internal/store"
)

const pw = "correct-horse-battery"

// 08:00 in Europe/Amsterdam on 2026-10-28.
var fixedNow = time.Date(2026, 10, 28, 7, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	auth.Iterations = 1000 // fast hashing in tests only
	os.Exit(m.Run())
}

func seedDoc() (*store.Document, error) {
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return nil, err
	}
	d := &store.Document{
		SchemaVersion: store.SchemaVersion, Revision: 1,
		Settings: store.Settings{OfficeName: "HQ", FloorName: "1", Timezone: "Europe/Amsterdam",
			WindowDays: 60, OpenHour: 7, CloseHour: 19, BackupRetention: 20},
		Desks:    []store.Desk{{ID: "D-01", Label: "A1", Active: true}, {ID: "D-02", Label: "A2", Active: true}},
		Bookings: []store.Booking{},
		Employees: []store.Employee{
			{ID: "e-admin", Username: "admin", Name: "Admin", Role: "admin", PasswordHash: hash, Active: true},
		},
	}
	for i := 1; i <= 10; i++ {
		d.Employees = append(d.Employees, store.Employee{ID: fmt.Sprintf("e%d", i), Username: fmt.Sprintf("user%d", i),
			Name: fmt.Sprintf("User %d", i), Role: "employee", PasswordHash: hash, Active: true})
	}
	return d, nil
}

type env struct {
	t    *testing.T
	path string
	st   *store.JSONStore
	srv  *Server
	ts   *httptest.Server
}

func newEnv(t *testing.T, path string) *env {
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "deskbook.json")
	}
	st, err := store.Open(path, store.Options{Seed: seedDoc})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(st, nil)
	srv.Now = func() time.Time { return fixedNow }
	e := &env{t: t, path: path, st: st, srv: srv, ts: httptest.NewServer(srv.Handler())}
	t.Cleanup(e.stop)
	return e
}

func (e *env) stop() {
	e.ts.Close()
	e.st.Close()
}

type client struct {
	e    *env
	http *http.Client
	csrf string
}

func (e *env) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar}}
}

func (e *env) login(user string) *client {
	e.t.Helper()
	c := e.anon()
	var out struct{ CSRFToken string }
	if code := c.do("POST", "/api/login", map[string]string{"username": user, "password": pw}, &out); code != 200 {
		e.t.Fatalf("login %s: %d", user, code)
	}
	c.csrf = out.CSRFToken
	return c
}

func (c *client) do(method, path string, body any, out any) int {
	c.e.t.Helper()
	var rd io.Reader
	if s, ok := body.(string); ok {
		rd = bytes.NewBufferString(s)
	} else if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.ts.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.csrf != "" {
		req.Header.Set(csrfHeader, c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func book(desk, date string, s, e int) map[string]any {
	return map[string]any{"deskId": desk, "date": date, "startHour": s, "endHour": e}
}

type bookingResp struct {
	Booking struct {
		ID         string `json:"id"`
		EmployeeID string `json:"employeeId"`
		Status     string `json:"status"`
	} `json:"booking"`
}

func TestAuthenticationRequired(t *testing.T) {
	e := newEnv(t, "")
	a := e.anon()
	for _, p := range []string{"/api/desks", "/api/bookings", "/api/config", "/api/admin/employees"} {
		if code := a.do("GET", p, nil, nil); code != 401 {
			t.Errorf("%s: want 401 got %d", p, code)
		}
	}
	if code := a.do("POST", "/api/bookings", book("D-01", "2026-10-29", 9, 10), nil); code != 401 {
		t.Errorf("anonymous booking: %d", code)
	}
	var s struct {
		User      *employeeView `json:"user"`
		CSRFToken string        `json:"csrfToken"`
	}
	if code := a.do("GET", "/api/session", nil, &s); code != 200 || s.User != nil || s.CSRFToken != "" {
		t.Errorf("signed-out session status: %d %+v", code, s)
	}
	c := e.login("user1")
	if code := c.do("GET", "/api/session", nil, &s); code != 200 || s.User == nil || s.User.ID != "e1" || s.CSRFToken != c.csrf {
		t.Errorf("signed-in session status: %d %+v", code, s)
	}
}

func TestLoginFailuresAndRateLimit(t *testing.T) {
	e := newEnv(t, "")
	a := e.anon()
	if code := a.do("POST", "/api/login", map[string]string{"username": "nobody", "password": pw}, nil); code != 401 {
		t.Fatalf("unknown user: %d", code)
	}
	for i := 0; i < 5; i++ {
		if code := a.do("POST", "/api/login", map[string]string{"username": "user1", "password": "wrong-password"}, nil); code != 401 {
			t.Fatalf("wrong password: %d", code)
		}
	}
	if code := a.do("POST", "/api/login", map[string]string{"username": "user1", "password": pw}, nil); code != 429 {
		t.Fatalf("want 429 after repeated failures, got %d", code)
	}
}

func TestCSRFAndOriginEnforced(t *testing.T) {
	e := newEnv(t, "")
	c := e.login("user1")
	token := c.csrf
	c.csrf = ""
	if code := c.do("POST", "/api/bookings", book("D-01", "2026-10-29", 9, 10), nil); code != 403 {
		t.Fatalf("missing CSRF: want 403 got %d", code)
	}
	c.csrf = "forged"
	if code := c.do("POST", "/api/bookings", book("D-01", "2026-10-29", 9, 10), nil); code != 403 {
		t.Fatalf("bad CSRF: want 403 got %d", code)
	}
	c.csrf = token
	b, _ := json.Marshal(book("D-01", "2026-10-29", 9, 10))
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/bookings", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, token)
	req.Header.Set("Origin", "http://evil.example")
	resp, _ := c.http.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("cross-origin: want 403 got %d", resp.StatusCode)
	}
}

func TestClientCannotChooseEmployee(t *testing.T) {
	e := newEnv(t, "")
	c := e.login("user1")
	body := book("D-01", "2026-10-29", 9, 10)
	body["employeeId"] = "e2"
	if code := c.do("POST", "/api/bookings", body, nil); code != 400 {
		t.Fatalf("unknown field should be rejected, got %d", code)
	}
	var out bookingResp
	if code := c.do("POST", "/api/bookings", book("D-01", "2026-10-29", 9, 10), &out); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if out.Booking.EmployeeID != "e1" {
		t.Fatalf("booking owner taken from body: %s", out.Booking.EmployeeID)
	}
}

func TestMalformedRequests(t *testing.T) {
	e := newEnv(t, "")
	c := e.login("user1")
	for _, body := range []string{"{", "[]", `{"deskId":"D-01"} {}`, `{"deskId":1}`} {
		if code := c.do("POST", "/api/bookings", body, nil); code != 400 {
			t.Errorf("%q: want 400 got %d", body, code)
		}
	}
}

func TestOverlapBackToBackAndRanges(t *testing.T) {
	e := newEnv(t, "")
	u1, u2 := e.login("user1"), e.login("user2")
	steps := []struct {
		c    *client
		body map[string]any
		want int
	}{
		{u1, book("D-01", "2026-10-29", 9, 12), 201},
		{u2, book("D-01", "2026-10-29", 11, 13), 409}, // overlap
		{u2, book("D-01", "2026-10-29", 8, 10), 409},  // overlap
		{u2, book("D-01", "2026-10-29", 12, 14), 201}, // back-to-back after
		{u2, book("D-01", "2026-10-29", 7, 9), 201},   // back-to-back before
		{u1, book("D-02", "2026-10-29", 10, 11), 409}, // u1 already sits at D-01 then
		{u2, book("D-02", "2026-10-29", 14, 13), 400}, // end before start
		{u2, book("D-02", "2026-10-29", 13, 13), 400}, // zero length
		{u2, book("D-02", "2026-10-27", 9, 10), 422},  // past
		{u2, book("D-02", "2026-12-26", 9, 10), 201},  // day 60
		{u2, book("D-02", "2026-12-27", 9, 10), 422},  // day 61
		{u2, book("D-02", "2026-10-29", 6, 8), 422},   // before opening
	}
	for i, s := range steps {
		if code := s.c.do("POST", "/api/bookings", s.body, nil); code != s.want {
			t.Errorf("step %d %v: want %d got %d", i, s.body, s.want, code)
		}
	}
}

func TestConcurrentBookingsOnlyOneWins(t *testing.T) {
	e := newEnv(t, "")
	var clients []*client
	for i := 1; i <= 10; i++ {
		clients = append(clients, e.login(fmt.Sprintf("user%d", i)))
	}
	const perUser = 5
	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make(chan int, len(clients)*perUser)
	for _, c := range clients {
		for j := 0; j < perUser; j++ {
			wg.Add(1)
			go func(c *client) {
				defer wg.Done()
				<-start
				codes <- c.do("POST", "/api/bookings", book("D-01", "2026-10-29", 10, 12), nil)
			}(c)
		}
	}
	close(start)
	wg.Wait()
	close(codes)
	count := map[int]int{}
	for c := range codes {
		count[c]++
	}
	if count[201] != 1 || count[409] != len(clients)*perUser-1 {
		t.Fatalf("want exactly one 201 and the rest 409, got %v", count)
	}
	d := e.st.Snapshot()
	if len(d.Bookings) != 1 {
		t.Fatalf("want 1 stored booking, got %d", len(d.Bookings))
	}
	// The persisted file agrees with memory.
	disk, err := store.VerifyFile(e.path)
	if err != nil || len(disk.Bookings) != 1 {
		t.Fatalf("disk: %v %v", err, disk)
	}
}

func TestConcurrentNonOverlappingAllSucceed(t *testing.T) {
	e := newEnv(t, "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 1; i <= 10; i++ {
		c := e.login(fmt.Sprintf("user%d", i))
		wg.Add(1)
		go func(h int) {
			defer wg.Done()
			if c.do("POST", "/api/bookings", book("D-01", "2026-10-29", 7+h, 8+h), nil) == 201 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}(i - 1)
	}
	wg.Wait()
	if ok != 10 {
		t.Fatalf("want 10 back-to-back bookings, got %d", ok)
	}
}

func TestCancellationAndOwnership(t *testing.T) {
	e := newEnv(t, "")
	u1, u2, admin := e.login("user1"), e.login("user2"), e.login("admin")
	var b bookingResp
	u1.do("POST", "/api/bookings", book("D-01", "2026-10-29", 9, 10), &b)
	id := b.Booking.ID
	if code := u2.do("POST", "/api/bookings/"+id+"/cancel", nil, nil); code != 403 {
		t.Fatalf("other employee cancel: %d", code)
	}
	if code := u2.do("PATCH", "/api/bookings/"+id, book("D-02", "2026-10-29", 9, 10), nil); code != 403 {
		t.Fatalf("other employee change: %d", code)
	}
	if code := u1.do("PATCH", "/api/bookings/"+id, book("D-01", "2026-10-29", 10, 12), nil); code != 200 {
		t.Fatalf("owner change: %d", code)
	}
	if code := u1.do("POST", "/api/bookings/"+id+"/cancel", nil, &b); code != 200 || b.Booking.Status != "cancelled" {
		t.Fatalf("owner cancel: %d %s", code, b.Booking.Status)
	}
	if code := u1.do("POST", "/api/bookings/"+id+"/cancel", nil, nil); code != 409 {
		t.Fatalf("double cancel: %d", code)
	}
	if code := u1.do("PATCH", "/api/bookings/"+id, book("D-01", "2026-10-29", 13, 14), nil); code != 409 {
		t.Fatalf("change cancelled booking: %d", code)
	}
	// The slot is free again after cancellation.
	u2.do("POST", "/api/bookings", book("D-01", "2026-10-29", 10, 12), &b)
	if code := admin.do("POST", "/api/bookings/"+b.Booking.ID+"/cancel", nil, nil); code != 200 {
		t.Fatalf("admin cancel other's booking: %d", code)
	}
	if code := u1.do("POST", "/api/bookings/nope/cancel", nil, nil); code != 404 {
		t.Fatalf("missing booking: %d", code)
	}
}

func TestCannotCancelEndedBooking(t *testing.T) {
	e := newEnv(t, "")
	u1 := e.login("user1")
	var b bookingResp
	u1.do("POST", "/api/bookings", book("D-01", "2026-10-28", 8, 9), &b)
	e.srv.Now = func() time.Time { return fixedNow.Add(2 * time.Hour) } // 10:00 office time
	if code := u1.do("POST", "/api/bookings/"+b.Booking.ID+"/cancel", nil, nil); code != 422 {
		t.Fatalf("cancel ended booking: %d", code)
	}
}

func TestAdminAuthorization(t *testing.T) {
	e := newEnv(t, "")
	u1, admin := e.login("user1"), e.login("admin")
	if code := u1.do("GET", "/api/admin/employees", nil, nil); code != 403 {
		t.Fatalf("employee listing employees: %d", code)
	}
	if code := u1.do("PATCH", "/api/admin/desks/D-01", map[string]any{"x": 10}, nil); code != 403 {
		t.Fatalf("employee moving desk: %d", code)
	}
	if code := admin.do("PATCH", "/api/admin/desks/D-01", map[string]any{"x": 10, "y": 20, "rotation": 90}, nil); code != 200 {
		t.Fatalf("admin moving desk: %d", code)
	}
	if code := admin.do("PATCH", "/api/admin/desks/D-01", map[string]any{"rotation": 45}, nil); code != 400 {
		t.Fatalf("bad rotation: %d", code)
	}
	if code := admin.do("PATCH", "/api/admin/employees/e-admin", map[string]any{"role": "employee"}, nil); code != 422 {
		t.Fatalf("self-demotion: %d", code)
	}
	if code := admin.do("POST", "/api/admin/employees", map[string]any{"username": "new.person", "name": "New Person",
		"role": "employee", "password": "short"}, nil); code != 400 {
		t.Fatalf("weak password: %d", code)
	}
	if code := admin.do("POST", "/api/admin/employees", map[string]any{"username": "new.person", "name": "New Person",
		"role": "employee", "password": pw}, nil); code != 201 {
		t.Fatalf("create employee: %d", code)
	}
	e.login("new.person")
	// Promote user1: role comes from server data, so it applies on the next request.
	if code := admin.do("PATCH", "/api/admin/employees/e1", map[string]any{"role": "admin"}, nil); code != 200 {
		t.Fatal("promote")
	}
	if code := u1.do("GET", "/api/admin/employees", nil, nil); code != 200 {
		t.Fatalf("promoted user: %d", code)
	}
	// Deactivation ends sessions immediately and frees future bookings.
	u2 := e.login("user2")
	var b bookingResp
	u2.do("POST", "/api/bookings", book("D-02", "2026-10-30", 9, 10), &b)
	admin.do("PATCH", "/api/admin/employees/e2", map[string]any{"active": false}, nil)
	if code := u2.do("GET", "/api/desks", nil, nil); code != 401 {
		t.Fatalf("deactivated user still signed in: %d", code)
	}
	if i, _ := e.st.Snapshot().BookingByID(b.Booking.ID); e.st.Snapshot().Bookings[i].Status != "cancelled" {
		t.Fatal("deactivated user's booking not cancelled")
	}
	if code := admin.do("PUT", "/api/admin/settings", map[string]any{"officeName": "HQ", "floorName": "1",
		"timezone": "Nowhere/City", "windowDays": 60, "openHour": 7, "closeHour": 19, "backupRetention": 20}, nil); code != 422 {
		t.Fatalf("invalid timezone accepted: %d", code)
	}
}

func TestPersistenceAfterServerRestart(t *testing.T) {
	e := newEnv(t, "")
	var b bookingResp
	if code := e.login("user1").do("POST", "/api/bookings", book("D-01", "2026-10-29", 9, 10), &b); code != 201 {
		t.Fatal(code)
	}
	path := e.path
	e.stop()

	e2 := newEnv(t, path)
	var out struct {
		Bookings []struct {
			ID string `json:"id"`
		} `json:"bookings"`
	}
	e2.login("user3").do("GET", "/api/bookings", nil, &out)
	if len(out.Bookings) != 1 || out.Bookings[0].ID != b.Booking.ID {
		t.Fatalf("booking lost across restart: %+v", out)
	}
}

// failingStore simulates a disk failure: Update never commits.
type failingStore struct{ store.Store }

func (failingStore) Update(func(*store.Document) error) (*store.Document, error) {
	return nil, errors.New("disk full")
}

func TestStorageFailureIsNotReportedAsSuccess(t *testing.T) {
	e := newEnv(t, "")
	c := e.login("user1")
	e.srv.Store = failingStore{e.st}
	var out map[string]map[string]string
	if code := c.do("POST", "/api/bookings", book("D-01", "2026-10-29", 9, 10), &out); code != 500 {
		t.Fatalf("want 500, got %d", code)
	}
	if out["error"]["code"] != "storage" {
		t.Fatalf("error body: %v", out)
	}
	if len(e.st.Snapshot().Bookings) != 0 {
		t.Fatal("booking recorded despite failure")
	}
}
