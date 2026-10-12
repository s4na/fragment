//go:build !darwin && !linux

package storage

import (
	"context"
	"github.com/s4na/fragment/internal/app"
)

func (s *Store) lock(context.Context) (func(), error) {
	return nil, app.Fail("unsupported", "このOSのファイルロックは未対応です")
}
