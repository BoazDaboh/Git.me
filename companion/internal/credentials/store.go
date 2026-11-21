package credentials

import "errors"

var ErrMissing = errors.New("credential not found")

type Vault interface {
	Read() (string, error)
	Write(string) error
	Delete() error
}
type Store struct{ Target string }

func New() *Store { return &Store{Target: "GitMe/GitHub/api.github.com"} }
