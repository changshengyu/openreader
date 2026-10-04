//go:build !linux && !darwin

package rootedfs

import "os"

func renameWriteNoReplace(parent *os.File, from, to string) error {
	return ErrUnsafePath
}

func renameBetweenNoReplace(fromParent *os.File, from string, toParent *os.File, to string) error {
	return ErrUnsafePath
}
