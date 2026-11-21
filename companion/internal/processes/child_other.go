//go:build !windows

package processes

import (
	"errors"
	"io"
)

type child struct{}

func launch(string, string, io.Writer) (*child, error) {
	return nil, errors.New("process controls currently support Windows only")
}
func (*child) pid() int      { return 0 }
func (*child) wait() error   { return nil }
func (*child) exitCode() int { return 0 }
func (*child) kill() error   { return nil }
func (*child) close()        {}
