package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDoc() *Document {
	return &Document{
		SchemaVersion: SchemaVersion,
		Revision:      1,
		Settings: Settings{OfficeName: "HQ", FloorName: "1", Timezone: "Europe/Amsterdam",
			WindowDays: 60, OpenHour: 7, CloseHour: 19, BackupRetention: 5},
		Desks:     []Desk{{ID: "D-01", Label: "A1", Active: true}},
		Employees: []Employee{{ID: "e1", Username: "ann", Name: "Ann", Role: "employee", Active: true}},
		Bookings:  []Booking{},
	}
}

func seed() (*Document, error) { return testDoc(), nil }

func openTemp(t *testing.T) (*JSONStore, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "deskbook.json")
	s, err := Open(p, Options{Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func addBooking(id string, start, end int) func(*Document) error {
	return func(d *Document) error {
		d.Bookings = append(d.Bookings, Booking{ID: id, DeskID: "D-01", EmployeeID: "e1",
			Date: "2026-10-29", StartHour: start, EndHour: end, Status: "active"})
		return nil
	}
}

func TestSeedsWhenMissing(t *testing.T) {
	s, p := openTemp(t)
	if _, err := os.Stat(p); err != nil {
		t.Fatal("data file not created:", err)
	}
	if s.Snapshot().Revision != 1 {
		t.Fatal("unexpected revision")
	}
}

func TestMissingFileWithBackupsRefusesToReseed(t *testing.T) {
	s, p := openTemp(t)
	if _, err := s.Update(addBooking("b1", 9, 10)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	os.Remove(p)
	_, err := Open(p, Options{Seed: seed})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	s, p := openTemp(t)
	if _, err := s.Update(addBooking("b1", 9, 10)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	d := s2.Snapshot()
	if d.Revision != 2 || len(d.Bookings) != 1 || d.Bookings[0].ID != "b1" {
		t.Fatalf("booking not persisted: %+v", d)
	}
}

func TestRejectsMalformedAndWrongSchema(t *testing.T) {
	good, _ := Encode(testDoc())
	cases := map[string][]byte{
		"garbage":        []byte("not json"),
		"truncated":      good[:len(good)/2],
		"empty":          {},
		"no version":     []byte(`{"revision":1}`),
		"future version": bytes.Replace(good, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 2`), 1),
		"unknown field":  bytes.Replace(good, []byte(`"revision": 1`), []byte(`"revision": 1, "extra": true`), 1),
		"trailing":       append(append([]byte{}, good...), []byte("{}")...),
		"bad timezone":   bytes.Replace(good, []byte("Europe/Amsterdam"), []byte("Mars/Base"), 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "deskbook.json")
			os.WriteFile(p, data, 0o600)
			s, err := Open(p, Options{Seed: seed})
			if err == nil {
				s.Close()
				t.Fatal("expected rejection")
			}
			if name == "future version" && !errors.Is(err, ErrSchemaVersion) {
				t.Fatalf("want ErrSchemaVersion, got %v", err)
			}
			after, _ := os.ReadFile(p)
			if !bytes.Equal(after, data) {
				t.Fatal("rejected file was modified")
			}
		})
	}
}

func TestRejectsOverlappingBookingsInFile(t *testing.T) {
	d := testDoc()
	addBooking("b1", 9, 12)(d)
	addBooking("b2", 11, 13)(d)
	if err := Validate(d); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	d.Bookings[1].StartHour = 12 // back-to-back is fine
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedWriteKeepsOriginal(t *testing.T) {
	s, p := openTemp(t)
	before, _ := os.ReadFile(p)
	s.beforeRename = func(tmp string) error { return errors.New("simulated power loss") }
	if _, err := s.Update(addBooking("b1", 9, 10)); err == nil {
		t.Fatal("expected write failure to be reported")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("original file changed after failed write")
	}
	if len(s.Snapshot().Bookings) != 0 {
		t.Fatal("failed write was committed in memory")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".deskbook-*.tmp")); len(m) != 0 {
		t.Fatalf("temp files left behind: %v", m)
	}
	s.beforeRename = nil
	if _, err := s.Update(addBooking("b1", 9, 10)); err != nil {
		t.Fatal("store unusable after failed write:", err)
	}
}

func TestCrashLeftoverTempIgnored(t *testing.T) {
	s, p := openTemp(t)
	s.Close()
	dir := filepath.Dir(p)
	stray := filepath.Join(dir, ".deskbook-123.tmp")
	os.WriteFile(stray, []byte(`{"schemaVersion":1,"rev`), 0o600) // half-written temp
	s2, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Fatal("stray temp file not cleaned up")
	}
}

func TestFnErrorWritesNothing(t *testing.T) {
	s, p := openTemp(t)
	before, _ := os.ReadFile(p)
	_, err := s.Update(func(d *Document) error {
		d.Bookings = append(d.Bookings, Booking{ID: "x"})
		return errors.New("rule failed")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("file changed")
	}
}

func TestInvalidResultNotWritten(t *testing.T) {
	s, _ := openTemp(t)
	_, err := s.Update(func(d *Document) error {
		addBooking("b1", 9, 12)(d)
		return addBooking("b2", 10, 11)(d)
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestRefusesWriteWhenDiskCorruptedWhileRunning(t *testing.T) {
	s, p := openTemp(t)
	os.WriteFile(p, []byte("{broken"), 0o600)
	if _, err := s.Update(addBooking("b1", 9, 10)); err == nil {
		t.Fatal("write accepted on top of corrupt file")
	}
}

func TestSecondInstanceIsLocked(t *testing.T) {
	_, p := openTemp(t)
	if _, err := Open(p, Options{Seed: seed}); !errors.Is(err, ErrLocked) {
		t.Fatalf("want ErrLocked, got %v", err)
	}
}

func TestBackupsRotateAndRecover(t *testing.T) {
	s, p := openTemp(t)
	tick := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { tick = tick.Add(time.Second); return tick }
	for i := 0; i < 8; i++ {
		if _, err := s.Update(addBooking("b"+string(rune('a'+i)), 7+i, 8+i)); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.ListBackups()
	if len(list) != 5 { // BackupRetention
		t.Fatalf("want 5 backups, got %d", len(list))
	}
	if list[0].Revision != 9 {
		t.Fatalf("newest backup revision %d", list[0].Revision)
	}
	s.Close()

	// Corrupt both the data file and the newest backup.
	os.WriteFile(p, []byte("garbage"), 0o600)
	os.WriteFile(filepath.Join(BackupDir(p), list[0].Name), []byte("garbage"), 0o600)
	if _, err := Open(p, Options{Seed: seed}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	src, doc, err := LatestValidBackup(p)
	if err != nil || doc.Revision != 8 || !strings.HasSuffix(src, list[1].Name) {
		t.Fatalf("latest valid backup: %v %v %v", src, doc, err)
	}
	if _, err := Restore(p, src); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(p + ".before-restore-*"); len(m) != 1 {
		t.Fatal("corrupt file was not preserved before restore")
	}
	s2, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.Snapshot().Revision != 8 || len(s2.Snapshot().Bookings) != 7 {
		t.Fatal("restored data wrong")
	}
}

func TestRestoreRejectsInvalidBackupAndRunningServer(t *testing.T) {
	s, p := openTemp(t)
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte(`{"schemaVersion":99}`), 0o600)
	if _, err := Restore(p, bad); !errors.Is(err, ErrLocked) {
		t.Fatalf("restore while running: want ErrLocked, got %v", err)
	}
	s.Close()
	if _, err := Restore(p, bad); !errors.Is(err, ErrSchemaVersion) {
		t.Fatalf("want ErrSchemaVersion, got %v", err)
	}
}
