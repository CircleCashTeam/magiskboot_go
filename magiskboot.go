package magiskboot

import (
	"os"
)

const (
	HEADER_FILE     = "header"
	KERNEL_FILE     = "kernel"
	RAMDISK_FILE    = "ramdisk.cpio"
	VND_RAMDISK_DIR = "vendor_ramdisk"
	SECOND_FILE     = "second"
	EXTRA_FILE      = "extra"
	KER_DTB_FILE    = "kernel_dtb"
	RECV_DTBO_FILE  = "recovery_dtbo"
	DTB_FILE        = "dtb"
	BOOTCONFIG_FILE = "bootconfig"
	NEW_BOOT        = "new-boot.img"
)

func CheckEnv(key string) bool {
	value, ret := os.LookupEnv(key)
	if ret {
		if value == "true" {
			return true
		}
	}
	return false
}
