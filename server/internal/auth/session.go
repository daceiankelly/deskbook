package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"time"
)

// Sessions are held in memory, keyed by SHA-256 of the cookie token, so a
// memory dump or log line never contains a usable token. A server restart
// signs everyone out; bookings are unaffected.
type Session struct {
	EmployeeID string
	CSRF       string
	Expires    time.Time
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	TTL      time.Duration
	Now      func() time.Time
}

func NewManager(ttl time.Duration) *Manager {
	return &Manager{sessions: map[string]*Session{}, TTL: ttl, Now: time.Now}
}

func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func key(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (m *Manager) Create(employeeID string) (token string, s Session) {
	token = RandomToken()
	s = Session{EmployeeID: employeeID, CSRF: RandomToken(), Expires: m.Now().Add(m.TTL)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[key(token)] = &s
	m.gcLocked()
	return token, s
}

// Get returns the session and slides its expiry.
func (m *Manager) Get(token string) (Session, bool) {
	if token == "" {
		return Session{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[key(token)]
	if !ok {
		return Session{}, false
	}
	now := m.Now()
	if now.After(s.Expires) {
		delete(m.sessions, key(token))
		return Session{}, false
	}
	s.Expires = now.Add(m.TTL)
	return *s, true
}

func (m *Manager) Delete(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, key(token))
}

func (m *Manager) DeleteForEmployee(employeeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.sessions {
		if s.EmployeeID == employeeID {
			delete(m.sessions, k)
		}
	}
}

func (m *Manager) gcLocked() {
	now := m.Now()
	for k, s := range m.sessions {
		if now.After(s.Expires) {
			delete(m.sessions, k)
		}
	}
}

func CSRFMatches(s Session, header string) bool {
	return header != "" && subtle.ConstantTimeCompare([]byte(s.CSRF), []byte(header)) == 1
}

// Limiter blocks a username+IP pair after repeated failed logins.
type Limiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	Max      int
	Window   time.Duration
	Now      func() time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{failures: map[string][]time.Time{}, Max: max, Window: window, Now: time.Now}
}

func (l *Limiter) recent(k string) []time.Time {
	cut := l.Now().Add(-l.Window)
	kept := l.failures[k][:0]
	for _, t := range l.failures[k] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, k)
	} else {
		l.failures[k] = kept
	}
	return kept
}

func (l *Limiter) Blocked(k string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(k)) >= l.Max
}

func (l *Limiter) Fail(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[k] = append(l.recent(k), l.Now())
}

func (l *Limiter) Reset(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, k)
}
