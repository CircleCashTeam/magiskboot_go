//go:build !windows
// +build !windows

package stub

import (
	"golang.org/x/sys/unix"
)

// Stub functions link to unix libraries

func Major(dev uint64) uint32 {
	return unix.Major(dev)
}

func Minor(dev uint64) uint32 {
	return unix.Minor(dev)
}

func Mkdev(major, minor uint32) uint64 {
	return unix.Mkdev(major, minor)
}

func Mknod(path string, mode uint32, dev int) error {
	return unix.Mknod(path, mode, dev)
}

type Stat_t struct {
	unix.Stat_t
}

func Stat(path string, stat *Stat_t) error {
	return unix.Stat(path, &stat.Stat_t)
}
