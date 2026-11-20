//go:build darwin || linux

package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Every Flow process takes the same advisory lock before reading a snapshot
// and committing a mutation. SQLite transactions still provide crash atomicity.
func diskLock(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ".flow.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		if (!errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN)) || time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("workspace_lock: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
