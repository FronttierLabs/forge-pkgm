package transaction

import (
	"os"
	"path/filepath"
	"syscall"
)

// Lock is an exclusive advisory lock on the local package database.
type Lock struct {
	f *os.File
}

// AcquireLock takes an exclusive flock on <dbpath>/forge.lck. It blocks
// until the lock is available.
func AcquireLock(dbpath string) (*Lock, error) {
	if err := os.MkdirAll(dbpath, 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(filepath.Join(dbpath, "forge.lck"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}

	return &Lock{f: f}, nil
}

// Close releases the lock and closes the lock file.
func (l *Lock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
