// Package store persists the deskbook Document.
//
// IMPORTANT: exactly one deskbook process may own a data file. The in-process
// mutex below serialises writers inside that process only; an OS-level lock
// file (data file + ".lock") makes a second process on the same host fail
// fast at startup. Do not place the data file on a network share used by
// several servers: lock semantics on SMB/NFS are not reliable.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store is the persistence abstraction the API depends on. A different
// backend (e.g. a database) can implement it without touching handlers.
type Store interface {
	// Snapshot returns a private copy of the current committed document.
	Snapshot() *Document
	// Update runs fn against the latest data inside one critical section:
	// read latest -> fn checks & mutates -> validate -> durable write -> commit.
	// If fn or the write fails nothing is committed and the error is returned.
	Update(fn func(doc *Document) error) (*Document, error)
	Backup() (string, error)
	ListBackups() ([]BackupInfo, error)
	Close() error
}

type BackupInfo struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Revision int64     `json:"revision"`
}

type Options struct {
	// Seed creates the initial document when no data file and no backups exist.
	Seed func() (*Document, error)
	Log  *slog.Logger
}

type JSONStore struct {
	mu        sync.Mutex
	path      string
	backupDir string
	doc       *Document
	lock      *fileLock
	log       *slog.Logger
	now       func() time.Time

	// beforeRename is a test hook that simulates a crash/failure between
	// writing the temp file and replacing the original.
	beforeRename func(tmp string) error
}

var _ Store = (*JSONStore)(nil)

func BackupDir(dataPath string) string { return filepath.Join(filepath.Dir(dataPath), "backups") }

// Open locks and loads the data file. A missing file is only initialised from
// Seed when there are also no backups; otherwise the operator must restore.
func Open(path string, opt Options) (*JSONStore, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	lk, err := acquireLock(path + ".lock")
	if err != nil {
		return nil, err
	}
	s := &JSONStore{path: path, backupDir: BackupDir(path), lock: lk, log: opt.Log, now: time.Now}
	if s.log == nil {
		s.log = slog.Default()
	}
	if err := os.MkdirAll(s.backupDir, 0o750); err != nil {
		lk.release()
		return nil, err
	}
	s.removeStaleTemps()

	doc, err := readDocument(path)
	if errors.Is(err, ErrNotFound) {
		backups, _ := s.ListBackups()
		if len(backups) > 0 || opt.Seed == nil {
			lk.release()
			return nil, fmt.Errorf("%w: %s (backups exist in %s; run `deskbook recover` or `deskbook restore <file>`)", ErrNotFound, path, s.backupDir)
		}
		if doc, err = opt.Seed(); err == nil {
			if err = Validate(doc); err == nil {
				err = writeAtomic(path, doc, nil)
			}
		}
		if err == nil {
			s.log.Info("initialised new data file", "path", path)
		}
	}
	if err != nil {
		lk.release()
		return nil, err
	}
	s.doc = doc
	return s, nil
}

func (s *JSONStore) Path() string { return s.path }

func (s *JSONStore) Snapshot() *Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doc.Clone()
}

func (s *JSONStore) Update(fn func(doc *Document) error) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Always start from what is on disk so every decision uses the latest
	// persisted data. If the file has become unreadable we refuse to write.
	cur, err := readDocument(s.path)
	if err != nil {
		return nil, fmt.Errorf("refusing write, current data unreadable: %w", err)
	}
	if cur.Revision != s.doc.Revision {
		s.log.Warn("data file changed outside this process; using on-disk version",
			"memory", s.doc.Revision, "disk", cur.Revision)
	}
	work := cur.Clone()
	if err := fn(work); err != nil {
		return nil, err
	}
	work.SchemaVersion = SchemaVersion
	work.Revision = cur.Revision + 1
	work.UpdatedAt = s.now().UTC()
	if err := Validate(work); err != nil {
		return nil, err
	}
	if err := writeAtomic(s.path, work, s.beforeRename); err != nil {
		return nil, fmt.Errorf("write failed, change not saved: %w", err)
	}
	s.doc = work
	// The change is durable at this point; a backup failure is logged, not fatal.
	if _, err := s.backupLocked(work); err != nil {
		s.log.Error("backup after write failed", "err", err)
	}
	return work.Clone(), nil
}

func (s *JSONStore) Backup() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backupLocked(s.doc)
}

func (s *JSONStore) backupLocked(doc *Document) (string, error) {
	name := fmt.Sprintf("deskbook-%s-r%08d.json", s.now().UTC().Format("20060102T150405.000Z"), doc.Revision)
	p := filepath.Join(s.backupDir, name)
	if err := writeAtomic(p, doc, nil); err != nil {
		return "", err
	}
	s.prune(doc.Settings.BackupRetention)
	return name, nil
}

func (s *JSONStore) prune(keep int) {
	list, err := s.ListBackups()
	if err != nil || len(list) <= keep {
		return
	}
	for _, b := range list[keep:] { // list is newest first
		if err := os.Remove(filepath.Join(s.backupDir, b.Name)); err != nil {
			s.log.Warn("could not prune backup", "name", b.Name, "err", err)
		}
	}
}

// ListBackups returns backups newest first.
func (s *JSONStore) ListBackups() ([]BackupInfo, error) { return listBackups(s.backupDir) }

func listBackups(dir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []BackupInfo
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, "deskbook-") || !strings.HasSuffix(n, ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		b := BackupInfo{Name: n, Size: info.Size(), Modified: info.ModTime()}
		if i := strings.LastIndex(n, "-r"); i > 0 {
			b.Revision, _ = strconv.ParseInt(strings.TrimSuffix(n[i+2:], ".json"), 10, 64)
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

func (s *JSONStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	err := s.lock.release()
	s.lock = nil
	return err
}

// removeStaleTemps deletes temp files left by a crash between write and rename.
// They are never read: the original file is still the committed state.
func (s *JSONStore) removeStaleTemps() {
	for _, dir := range []string{filepath.Dir(s.path), s.backupDir} {
		matches, _ := filepath.Glob(filepath.Join(dir, ".deskbook-*.tmp"))
		for _, m := range matches {
			s.log.Warn("removing temp file from interrupted write", "file", m)
			_ = os.Remove(m)
		}
	}
}

func readDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, err
	}
	return Decode(data)
}

// writeAtomic writes doc to a temp file in the target directory, fsyncs and
// closes it, then renames it over the target, then fsyncs the directory.
// Readers therefore see either the old or the new file, never a partial one.
func writeAtomic(path string, doc *Document, beforeRename func(string) error) (err error) {
	data, err := Encode(doc)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".deskbook-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if beforeRename != nil {
		if err = beforeRename(tmp); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(dir)
}

// VerifyFile reads and fully validates a data or backup file.
func VerifyFile(path string) (*Document, error) { return readDocument(path) }

// LatestValidBackup returns the newest backup that passes full validation.
func LatestValidBackup(dataPath string) (string, *Document, error) {
	dir := BackupDir(dataPath)
	list, err := listBackups(dir)
	if err != nil {
		return "", nil, err
	}
	for _, b := range list {
		p := filepath.Join(dir, b.Name)
		if doc, err := readDocument(p); err == nil {
			return p, doc, nil
		}
	}
	return "", nil, errors.New("no valid backup found")
}

// Restore validates `from` and atomically installs it as the data file.
// It takes the instance lock, so it fails while the server is running.
// The previous data file (valid or not) is preserved alongside.
func Restore(dataPath, from string) (*Document, error) {
	dataPath, err := filepath.Abs(dataPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o750); err != nil {
		return nil, err
	}
	lk, err := acquireLock(dataPath + ".lock")
	if err != nil {
		return nil, err
	}
	defer lk.release()
	doc, err := readDocument(from)
	if err != nil {
		return nil, fmt.Errorf("backup rejected: %w", err)
	}
	if old, err := os.ReadFile(dataPath); err == nil {
		aside := fmt.Sprintf("%s.before-restore-%s", dataPath, time.Now().UTC().Format("20060102T150405Z"))
		if err := os.WriteFile(aside, old, 0o600); err != nil {
			return nil, fmt.Errorf("could not preserve current file: %w", err)
		}
	}
	if err := writeAtomic(dataPath, doc, nil); err != nil {
		return nil, err
	}
	return doc, nil
}
