//go:build !darwin && !linux

package desktop

import (
	"context"
	"os"
	"os/exec"

	"github.com/s4na/fragment/internal/app"
)

func unsupported() error {
	return app.Fail("unsupported", "バックグラウンド起動はmacOS/Linuxで利用できます")
}
func runtimeBase() (string, error)                       { return "", unsupported() }
func privateDir(string) error                            { return unsupported() }
func privateFile(string, int) (*os.File, error)          { return nil, unsupported() }
func lock(context.Context, string, bool) (func(), error) { return nil, unsupported() }
func detach(*exec.Cmd)                                   {}
func unavailable(error) bool                             { return false }
