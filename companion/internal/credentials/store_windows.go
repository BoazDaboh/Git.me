package credentials

import (
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

var library = windows.NewLazySystemDLL("advapi32.dll")
var credWrite = library.NewProc("CredWriteW")
var credRead = library.NewProc("CredReadW")
var credDelete = library.NewProc("CredDeleteW")
var credFree = library.NewProc("CredFree")

type credential struct {
	Flags          uint32
	Type           uint32
	TargetName     *uint16
	Comment        *uint16
	LastWritten    windows.Filetime
	BlobSize       uint32
	Blob           *byte
	Persist        uint32
	AttributeCount uint32
	Attributes     uintptr
	TargetAlias    *uint16
	UserName       *uint16
}

func (s *Store) Write(secret string) error {
	if len(secret) == 0 || len(secret) > 2560 {
		return errors.New("invalid credential size")
	}
	target, err := windows.UTF16PtrFromString(s.Target)
	if err != nil {
		return errors.New("invalid credential target")
	}
	name, _ := windows.UTF16PtrFromString("GitMe")
	data := []byte(secret)
	defer func() {
		for i := range data {
			data[i] = 0
		}
	}()
	value := credential{Type: 1, TargetName: target, BlobSize: uint32(len(data)), Blob: &data[0], Persist: 2, UserName: name}
	ok, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&value)), 0)
	runtime.KeepAlive(data)
	runtime.KeepAlive(target)
	runtime.KeepAlive(name)
	if ok == 0 {
		return errors.New("Windows credential storage could not save this connection")
	}
	return nil
}
func (s *Store) Read() (string, error) {
	target, err := windows.UTF16PtrFromString(s.Target)
	if err != nil {
		return "", errors.New("invalid credential target")
	}
	var value *credential
	ok, _, callErr := credRead.Call(uintptr(unsafe.Pointer(target)), 1, 0, uintptr(unsafe.Pointer(&value)))
	runtime.KeepAlive(target)
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", ErrMissing
		}
		return "", errors.New("Windows credential storage could not read this connection")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(value)))
	if value == nil || value.Blob == nil || value.BlobSize == 0 || value.BlobSize > 2560 {
		return "", errors.New("stored GitHub credential is invalid")
	}
	return string(unsafe.Slice(value.Blob, int(value.BlobSize))), nil
}
func (s *Store) Delete() error {
	target, err := windows.UTF16PtrFromString(s.Target)
	if err != nil {
		return errors.New("invalid credential target")
	}
	ok, _, callErr := credDelete.Call(uintptr(unsafe.Pointer(target)), 1, 0)
	runtime.KeepAlive(target)
	if ok == 0 && !errors.Is(callErr, windows.ERROR_NOT_FOUND) {
		return errors.New("Windows credential storage could not remove this connection")
	}
	return nil
}
