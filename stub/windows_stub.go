//go:build windows

package stub

import "log"

// Stub functions, always return 0

func Major(dev uint64) uint32 {
	log.Println("Windows stub Major called")
	return 0
}

func Minor(dev uint64) uint32 {
	log.Println("Windows stub Minor called")
	return 0
}

func Mkdev(major, minor uint32) uint64 {
	log.Println("Windows stub Mkdev called")
	return 0
}

func Mknod(path string, mode uint32, dev int) error {
	log.Println("Windows stub Mknod called")
	return nil
}

type Stat_t struct {
	//unix.Stat_t
	Rdev uint64
}

func Stat(path string, stat *Stat_t) error {
	log.Println("Windows stub Stat called")
	stat.Rdev = uint64(0)
	return nil
}
