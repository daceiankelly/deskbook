//go:build windows

package store

import (
	"fmt"
	"os"
	"syscall"
)

// fileLock opens the lock file with no sharing, so a second process (or a
// second Open in this process) cannot open it. Windows releases the handle
// when the process exits.
type fileLock struct{ f *os.File }

func acquireLock(path string) (*fileLock, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %s (%v)", ErrLocked, path, err)
	}
	return &fileLock{f: os.NewFile(uintptr(h), path)}, nil
}

func (l *fileLock) release() error { return l.f.Close() }

// NTFS does not support fsync on directories; the rename itself is atomic
// (MoveFileEx with MOVEFILE_REPLACE_EXISTING).
func syncDir(string) error { return nil }
