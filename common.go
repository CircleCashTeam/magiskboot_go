package magiskboot

func align_to(v uint64, a uint64) uint64 {
	return (v + a - 1) / a * a
}

func align_padding(v, a uint64) uint64 {
	return align_to(v, a) - v
}
