//go:build windows

package stub

// Stub functions, always return 0

func Major(dev uint64) uint32 {
	return 0
}

func Minor(dev uint64) uint32 {
	return 0
}

func Mkdev(major, minor uint32) uint64 {
	return 0
}

func Mknod(path string, mode uint32, dev int) error {
	return nil
}

type Stat_t struct {
	//unix.Stat_t
	Rdev uint64
}

func Stat(path string, stat *Stat_t) error {
	stat.Rdev = uint64(0)
	return nil
}
