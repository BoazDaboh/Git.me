//go:build !windows

package localupload

import "os"

func singleFile(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode().IsRegular()
}
