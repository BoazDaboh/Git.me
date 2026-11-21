//go:build !windows

package localgit

import (
	"context"
	"os/exec"
)

func hide(*exec.Cmd) {}

func runOwned(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = cmd.Process.Kill() })
	defer stop()
	return cmd.Wait()
}
