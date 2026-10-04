package rootedfs

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameWriteNoReplace(parent *os.File, from, to string) error {
	return renameBetweenNoReplace(parent, from, parent, to)
}

func renameBetweenNoReplace(fromParent *os.File, from string, toParent *os.File, to string) error {
	return unix.Renameat2(int(fromParent.Fd()), from, int(toParent.Fd()), to, unix.RENAME_NOREPLACE)
}
