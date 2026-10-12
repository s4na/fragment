//go:build darwin || linux

package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/s4na/fragment/internal/app"
	"golang.org/x/sys/unix"
)

func runtimeBase() (string, error) {
	base, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "fragment-"+strconv.Itoa(os.Geteuid())), nil
}

func privateDir(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || stat.Uid != uint32(os.Geteuid()) {
		return app.Fail("runtime_error", "起動管理フォルダの所有者または構成が不正です: "+path)
	}
	return os.Chmod(path, 0o700)
}

func privateFile(path string, flags int) (*os.File, error) {
	f, err := os.OpenFile(path, flags|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || stat.Uid != uint32(os.Geteuid()) {
			err = app.Fail("runtime_error", "起動管理ファイルの所有者または構成が不正です")
		}
	}
	if err == nil {
		err = f.Chmod(0o600)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func lock(ctx context.Context, path string, wait bool) (func(), error) {
	f, err := privateFile(path, os.O_CREATE|os.O_RDWR)
	if err != nil {
		return nil, err
	}
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
		}
		if !wait || (!errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR)) {
			f.Close()
			return nil, app.Fail("already_running", "起動管理のロックを取得できませんでした")
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, app.Fail("lock_timeout", "起動管理のロック待機期限を超えました")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func unavailable(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED)
}
