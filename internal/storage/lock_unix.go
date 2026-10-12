//go:build darwin || linux

package storage

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/s4na/fragment/internal/app"
	"golang.org/x/sys/unix"
)

func (s *Store) lock(ctx context.Context) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, s.LockTimeout)
	defer cancel()
	p, err := s.checked(".fragment/write.lock", false)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_RDWR, 0600)
	if err != nil {
		return nil, mapError(err)
	}
	for {
		if ctx.Err() != nil {
			f.Close()
			return nil, app.Fail("lock_timeout", "書き込みロックの待機期限を超えました")
		}
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
			f.Close()
			return nil, mapError(err)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, app.Fail("lock_timeout", "書き込みロックの待機期限を超えました")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
