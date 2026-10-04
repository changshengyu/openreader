//go:build !linux && !darwin

package rootedfs

import "os"

func renameWriteNoReplace(parent *os.File, from, to string) error {
	return ErrUnsafePath
}
