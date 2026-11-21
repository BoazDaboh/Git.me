//go:build !windows

package credentials

import "errors"

func (*Store) Read() (string, error) {
	return "", errors.New("credential connections currently require Windows")
}
func (*Store) Write(string) error {
	return errors.New("credential connections currently require Windows")
}
func (*Store) Delete() error { return errors.New("credential connections currently require Windows") }
