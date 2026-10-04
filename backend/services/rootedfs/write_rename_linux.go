package rootedfs

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameWriteNoReplace(parent *os.File, from, to string) error {
	return unix.Renameat2(int(parent.Fd()), from, int(parent.Fd()), to, unix.RENAME_NOREPLACE)
}
