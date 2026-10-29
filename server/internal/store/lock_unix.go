//go:build !windows

package store

import (
	"fmt"
	"os"
	"syscall"
)

// fileLock is an advisory flock held for the life of the process; the kernel
// releases it automatically if the process dies, so crashes leave no stale lock.
type fileLock struct{ f *os.File }

func acquireLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("%w: %s", ErrLocked, path)
	}
	return &fileLock{f: f}, nil
}

func (l *fileLock) release() error {
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
