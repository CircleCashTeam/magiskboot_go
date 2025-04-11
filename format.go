package magiskboot

import "bytes"

const (
	UNKNOWN = iota
	/* Boot formats */
	CHROMEOS
	AOSP
	AOSP_VENDOR
	DHTB
	BLOB
	/* Compression formats */
	GZIP
	ZOPFLI
	XZ
	LZMA
	BZIP2
	LZ4
	LZ4_LEGACY
	LZ4_LG
	/* Unsupported compression */
	LZOP
	/* Misc */
	MTK
	DTB
	ZIMAGE
)

type format_t int

func COMPRESSED(fmt format_t) bool {
	return ((fmt) >= GZIP && (fmt) < LZOP)
}

func COMPRESSED_ANY(fmt format_t) bool {
	return ((fmt) >= GZIP && (fmt) <= LZOP)
}

const (
	BOOT_MAGIC               = "ANDROID!"
	VENDOR_BOOT_MAGIC        = "VNDRBOOT"
	CHROMEOS_MAGIC           = "CHROMEOS"
	GZIP1_MAGIC              = "\x1f\x8b"
	GZIP2_MAGIC              = "\x1f\x9e"
	LZOP_MAGIC               = "\x89LZO"
	XZ_MAGIC                 = "\xfd7zXZ"
	BZIP_MAGIC               = "BZh"
	LZ4_LEG_MAGIC            = "\x02\x21\x4c\x18"
	LZ41_MAGIC               = "\x03\x21\x4c\x18"
	LZ42_MAGIC               = "\x04\x22\x4d\x18"
	MTK_MAGIC                = "\x88\x16\x88\x58"
	DTB_MAGIC                = "\xd0\x0d\xfe\xed"
	LG_BUMP_MAGIC            = "\x41\xa9\xe4\x67\x74\x4d\x1d\x1b\xa4\x29\xf2\xec\xea\x65\x52\x79"
	DHTB_MAGIC               = "\x44\x48\x54\x42\x01\x00\x00\x00"
	SEANDROID_MAGIC          = "SEANDROIDENFORCE"
	TEGRABLOB_MAGIC          = "-SIGNED-BY-SIGNBLOB-"
	NOOKHD_RL_MAGIC          = "Red Loader"
	NOOKHD_GL_MAGIC          = "Green Loader"
	NOOKHD_GR_MAGIC          = "Green Recovery"
	NOOKHD_EB_MAGIC          = "eMMC boot.img+secondloader"
	NOOKHD_ER_MAGIC          = "eMMC recovery.img+secondloader"
	NOOKHD_PRE_HEADER_SZ     = 1048576
	ACCLAIM_MAGIC            = "BauwksBoot"
	ACCLAIM_PRE_HEADER_SZ    = 262144
	AMONET_MICROLOADER_MAGIC = "microloader"
	AMONET_MICROLOADER_SZ    = 1024
	AVB_FOOTER_MAGIC         = "AVBf"
	AVB_MAGIC                = "AVB0"
	ZIMAGE_MAGIC             = "\x18\x28\x6f\x01"
)

func CheckFmt(buf []byte) format_t {
	CHECKED_MATCH := func(p string) bool {
		return bytes.Equal([]byte(p), buf[:len(p)])
	}

	if CHECKED_MATCH(CHROMEOS_MAGIC) {
		return CHROMEOS
	} else if CHECKED_MATCH(BOOT_MAGIC) {
		return AOSP
	} else if CHECKED_MATCH(VENDOR_BOOT_MAGIC) {
		return AOSP_VENDOR
	} else if CHECKED_MATCH(GZIP1_MAGIC) || CHECKED_MATCH(GZIP2_MAGIC) {
		return GZIP
	} else if CHECKED_MATCH(LZOP_MAGIC) {
		return LZOP
	} else if CHECKED_MATCH(XZ_MAGIC) {
		return XZ
	} else if (len(buf) >= 13 && bytes.Equal([]byte("\x5d\x00\x00"), buf[:3])) && (buf[12] == '\xff' || buf[12] == '\x00') {
		return LZMA
	} else if CHECKED_MATCH(BZIP_MAGIC) {
		return BZIP2
	} else if CHECKED_MATCH(LZ41_MAGIC) || CHECKED_MATCH(LZ42_MAGIC) {
		return LZ4
	} else if CHECKED_MATCH(LZ4_LEG_MAGIC) {
		return LZ4_LEGACY
	} else if CHECKED_MATCH(MTK_MAGIC) {
		return MTK
	} else if CHECKED_MATCH(DTB_MAGIC) {
		return DTB
	} else if CHECKED_MATCH(DHTB_MAGIC) {
		return DHTB
	} else if CHECKED_MATCH(TEGRABLOB_MAGIC) {
		return BLOB
	} else if len(buf) >= 0x28 && bytes.Equal(buf[0x24:len(ZIMAGE_MAGIC)+0x24], []byte(ZIMAGE_MAGIC)) {
		return ZIMAGE
	} else {
		return UNKNOWN
	}
}

func Fmt2Name(fmt format_t) string {
	switch fmt {
	case GZIP:
		return "gzip"
	case ZOPFLI:
		return "zopfli"
	case LZOP:
		return "lzop"
	case XZ:
		return "xz"
	case LZMA:
		return "lzma"
	case BZIP2:
		return "bzip2"
	case LZ4:
		return "lz4"
	case LZ4_LEGACY:
		return "lz4_legacy"
	case LZ4_LG:
		return "lz4_lg"
	case DTB:
		return "dtb"
	case ZIMAGE:
		return "zimage"
	default:
		return "raw"
	}
}

func Fmt2Ext(fmt format_t) string {
	switch fmt {
	case GZIP:
	case ZOPFLI:
		return ".gz"
	case LZOP:
		return ".lzo"
	case XZ:
		return ".xz"
	case LZMA:
		return ".lzma"
	case BZIP2:
		return ".bz2"
	case LZ4:
	case LZ4_LEGACY:
	case LZ4_LG:
		return ".lz4"
	default:
		return ""
	}
	return ""
}

func Name2Fmt(name string) format_t {
	switch name {
	case "gzip":
		return GZIP
	case "zopfli":
		return ZOPFLI
	case "xz":
		return XZ
	case "lzma":
		return LZMA
	case "bzip2":
		return BZIP2
	case "lz4":
		return LZ4
	case "lz4_legacy":
		return LZ4_LEGACY
	case "lz4_lg":
		return LZ4_LG
	default:
		return UNKNOWN
	}
}
