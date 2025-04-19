package magiskboot

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"unsafe"

	"github.com/edsrzf/mmap-go"
)

type MtkHdr struct {
	Magic   uint32
	Size    uint32
	Name    [32]byte
	Padding [472]byte
}

type DhtbHdr struct {
	Magic    [8]byte
	Checksum [40]uint8
	Size     uint32
	Padding  [460]byte
}

//go:packed
type BlobHdr struct {
	SecureMagic [20]byte
	Datalen     uint32
	Signature   uint32
	Magic       [16]byte
	HdrVersion  uint32
	HdrSize     uint32
	PartOffset  uint32
	NumParts    uint32
	Unknow      [7]uint32
	Name        [4]byte
	Offset      uint32
	Size        uint32
	Version     uint32
}

//go:packed
type ZimageHdr struct {
	Code   [9]uint32
	Magic  uint32
	Start  uint32
	End    uint32
	Endian uint32
}

const (
	AVB_FOOTER_MAGIC_LEN    = 4
	AVB_MAGIC_LEN           = 4
	AVB_RELEASE_STRING_SIZE = 48
)

//go:packed
type AvbFooter struct {
	Magic             [AVB_FOOTER_MAGIC_LEN]uint8
	VersionMajor      uint32
	VersionMinor      uint32
	OriginalImageSize uint64
	VbmetaOffset      uint64
	VbmetaSize        uint64
	Reserved          [28]byte
}

//go:packed
type AvbVBMetaImageHeader struct {
	Magic                       [AVB_MAGIC_LEN]uint8
	RequiredLibavbVersionMajor  uint32
	RequiredLibavbVersionMinor  uint32
	AuthenticationDataBlockSize uint64
	AuxiliaryDataBlockSize      uint64
	AlgorithmType               uint32
	HashOffset                  uint64
	HashSize                    uint64
	SignatureOffset             uint64
	SignatureSize               uint64
	PublicKeyOffset             uint64
	PublicKeySize               uint64
	PublicKeyMetadataOffset     uint64
	PublicKeyMetadataSize       uint64
	DescriptorsOffset           uint64
	DescriptorsSize             uint64
	RollbackIndex               uint64
	Flags                       uint32
	RollbackIndexLocation       uint32
	ReleaseString               [AVB_RELEASE_STRING_SIZE]byte
	Reserved                    [80]byte
}

const BOOT_MAGIC_SIZE = 8
const BOOT_NAME_SIZE = 16
const BOOT_ID_SIZE = 32
const BOOT_ARGS_SIZE = 512
const BOOT_EXTRA_ARGS_SIZE = 1024
const VENDOR_BOOT_ARGS_SIZE = 2048
const VENDOR_RAMDISK_NAME_SIZE = 32
const VENDOR_RAMDISK_TABLE_ENTRY_BOARD_ID_SIZE = 16

const VENDOR_RAMDISK_TYPE_NONE = 0
const VENDOR_RAMDISK_TYPE_PLATFORM = 1
const VENDOR_RAMDISK_TYPE_RECOVERY = 2
const VENDOR_RAMDISK_TYPE_DLKM = 3

type BootImgHdrV0Common struct {
	Magic       [BOOT_MAGIC_SIZE]byte
	KernelSize  uint32 // size in bytes
	KernelAddr  uint32 // physical load addr
	RamdiskSize uint32 // size in bytes
	RamdiskAddr uint32 // physical load addr
	SecondSize  uint32 // size in bytes
	SecondAddr  uint32 // physical load addr
} // 总大小: 8 + 6*4 = 32 字节

type BootImgHdrV0 struct {
	BootImgHdrV0Common
	TagsAddr      uint32
	PageSize      uint32 // or Unknown for Samsung
	HeaderVersion uint32 // or ExtraSize for Samsung
	OsVersion     uint32
	Name          [BOOT_NAME_SIZE]byte
	Cmdline       [BOOT_ARGS_SIZE]byte
	Id            [BOOT_ID_SIZE]byte
	ExtraCmdline  [BOOT_EXTRA_ARGS_SIZE]byte
} // 总大小: 32 + 4*4 + 16 + 512 + 32 + 1024 = 1632 字节

type BootImgHdrV1 struct {
	BootImgHdrV0
	RecoveryDtboSize   uint32
	RecoveryDtboOffset uint64
	HeaderSize         uint32
} // 总大小: 1632 + 4 + 8 + 4 = 1648 字节

type BootImgHdrV2 struct {
	BootImgHdrV1
	DtbSize uint32
	DtbAddr uint64
} // 总大小: 1648 + 4 + 8 = 1660 字节

type BootImgHdrPxa struct {
	BootImgHdrV0Common
	ExtraSize    uint32
	Unknown      uint32
	TagsAddr     uint32
	PageSize     uint32
	Name         [24]byte
	Cmdline      [BOOT_ARGS_SIZE]byte
	Id           [BOOT_ID_SIZE]byte
	ExtraCmdline [BOOT_EXTRA_ARGS_SIZE]byte
} // 总大小: 32 + 4*4 + 24 + 512 + 32 + 1024 = 1640 字节

/*
 * When the boot image header has a version of 3 - 4, the structure of the boot
 * image is as follows:
 *
 * +---------------------+
 * | boot header         | 4096 bytes
 * +---------------------+
 * | kernel              | m pages
 * +---------------------+
 * | ramdisk             | n pages
 * +---------------------+
 * | boot signature      | g pages
 * +---------------------+
 *
 * m = (kernel_size + 4096 - 1) / 4096
 * n = (ramdisk_size + 4096 - 1) / 4096
 * g = (signature_size + 4096 - 1) / 4096
 *
 * Page size is fixed at 4096 bytes.
 *
 * The structure of the vendor boot image is as follows:
 *
 * +------------------------+
 * | vendor boot header     | o pages
 * +------------------------+
 * | vendor ramdisk section | p pages
 * +------------------------+
 * | dtb                    | q pages
 * +------------------------+
 * | vendor ramdisk table   | r pages
 * +------------------------+
 * | bootconfig             | s pages
 * +------------------------+
 *
 * o = (2128 + page_size - 1) / page_size
 * p = (vendor_ramdisk_size + page_size - 1) / page_size
 * q = (dtb_size + page_size - 1) / page_size
 * r = (vendor_ramdisk_table_size + page_size - 1) / page_size
 * s = (vendor_bootconfig_size + page_size - 1) / page_size
 *
 * Note that in version 4 of the vendor boot image, multiple vendor ramdisks can
 * be included in the vendor boot image. The bootloader can select a subset of
 * ramdisks to load at runtime. To help the bootloader select the ramdisks, each
 * ramdisk is tagged with a type tag and a set of hardware identifiers
 * describing the board, soc or platform that this ramdisk is intended for.
 *
 * The vendor ramdisk section is consist of multiple ramdisk images concatenated
 * one after another, and vendor_ramdisk_size is the size of the section, which
 * is the total size of all the ramdisks included in the vendor boot image.
 *
 * The vendor ramdisk table holds the size, offset, type, name and hardware
 * identifiers of each ramdisk. The type field denotes the type of its content.
 * The vendor ramdisk names are unique. The hardware identifiers are specified
 * in the board_id field in each table entry. The board_id field is consist of a
 * vector of unsigned integer words, and the encoding scheme is defined by the
 * hardware vendor.
 *
 * For the different type of ramdisks, there are:
 *    - VENDOR_RAMDISK_TYPE_NONE indicates the value is unspecified.
 *    - VENDOR_RAMDISK_TYPE_PLATFORM ramdisks contain platform specific bits, so
 *      the bootloader should always load these into memory.
 *    - VENDOR_RAMDISK_TYPE_RECOVERY ramdisks contain recovery resources, so
 *      the bootloader should load these when booting into recovery.
 *    - VENDOR_RAMDISK_TYPE_DLKM ramdisks contain dynamic loadable kernel
 *      modules.
 *
 * Version 4 of the vendor boot image also adds a bootconfig section to the end
 * of the image. This section contains Boot Configuration parameters known at
 * build time. The bootloader is responsible for placing this section directly
 * after the generic ramdisk, followed by the bootconfig trailer, before
 * entering the kernel.
 */

type BootImgHdrV3 struct {
	Magic         [BOOT_MAGIC_SIZE]byte
	KernelSize    uint32
	RamdiskSize   uint32
	OsVersion     uint32
	HeaderSize    uint32
	Reserved      [4]uint32
	HeaderVersion uint32
	Cmdline       [BOOT_ARGS_SIZE + BOOT_EXTRA_ARGS_SIZE]byte
}

type BootImgHdrVndV3 struct {
	Magic         [BOOT_MAGIC_SIZE]byte
	HeaderVersion uint32
	PageSize      uint32
	KernelAddr    uint32
	RamdiskAddr   uint32
	RamdiskSize   uint32
	Cmdline       [VENDOR_BOOT_ARGS_SIZE]byte
	TagsAddr      uint32
	Name          [BOOT_NAME_SIZE]byte
	HeaderSize    uint32
	DtbSize       uint32
	DtbAddr       uint64
}

type BootImgHdrV4 struct {
	BootImgHdrV3
	SignatureSize uint32
}

type BootImgHdrVndV4 struct {
	BootImgHdrVndV3
	VendorRamdiskTableSize      uint32
	VendorRamdiskTableEntryNum  uint32
	VendorRamdiskTableEntrySize uint32
	BootconfigSize              uint32
}

type VendorRamdiskTableEntryV4 struct {
	RamdiskSize   uint32
	RamdiskOffset uint32
	RamdiskType   uint32
	RamdiskName   [VENDOR_RAMDISK_NAME_SIZE]byte
	BoardId       [VENDOR_RAMDISK_TABLE_ENTRY_BOARD_ID_SIZE]uint32
}

// Define dyn_img_hdr api
type DynImgHdrInterface interface {
	IsVendor() bool
	PageSize() uint32
	HeaderVersion() uint32
	SignatureSize() uint32
	VendorRamdiskTableSize() uint32
	VendorRamdiskTableEntryNum() uint32
	VendorRamdiskTableEntrySize() uint32

	HdrSize() uint64
	HdrSpace() uint64
	RawHdr() mmap.MMap

	Print()
	DumpHdrFile()
	LoadHdrFile()
}

type DynImgHdr struct {
	KernelSize   uint32
	RamdiskSize  uint32
	SecondSize   uint32
	ExtraSize    uint32
	OsVersion    uint32
	Name         string
	Cmdline      string
	Id           string
	ExtraCmdline string

	// v1/v2 specific
	RecoveryDtboSize   uint32
	RecoveryDtboOffset uint64
	HeaderSize         uint32
	DtbSize            uint32

	// v4 vendor specific
	VendorRamdiskTableSize      uint32
	VendorRamdiskTableEntryNum  uint32
	VendorRamdiskTableEntrySize uint32
	BootconfigSize              uint32

	// headers
	V2Hdr  BootImgHdrV2
	V4Hdr  BootImgHdrV4
	V4Vnd  BootImgHdrVndV4
	HdrPxa BootImgHdrPxa
	// No raw pointer need to be defined...
}

// Abstract
func (d *DynImgHdr) IsVendor() bool {
	return false
}

// Abstract
func (d *DynImgHdr) HdrSize() uint64 {
	return 0
}

func (d *DynImgHdr) HeaderVersion() uint32 {
	return 0
}

func (d *DynImgHdr) PageSize() uint32 {
	return 0
}

// v4 specific
func (d *DynImgHdr) SignatureSize() uint32 {
	return 0
}

func (d *DynImgHdr) HdrSpace() uint64 {
	return uint64(d.PageSize())
}

func (d *DynImgHdr) RawHdr() error {
	return errors.New("not impl yet")
}

func (d *DynImgHdr) Print() {

}

func (d *DynImgHdr) DumpHdrFile() {

}

func (d *DynImgHdr) LoadHdrFile() {

}

type DynImgHdrBoot struct {
	DynImgHdr
}

func (d *DynImgHdrBoot) IsVendor() bool {
	return false
}

type DynImgCommon struct {
	DynImgHdrBoot

	KernelSize  *uint32
	RamdiskSize *uint32
	SecondSize  *uint32
}

func (d *DynImgCommon) Init() {
	d.KernelSize = &d.V2Hdr.KernelSize
	d.RamdiskSize = &d.V2Hdr.RamdiskSize
	d.SecondSize = &d.V2Hdr.SecondSize
}

type DynImgV0 struct {
	DynImgCommon

	Raw BootImgHdrV0

	ExtraSize    *uint32
	OsVersion    *uint32
	Name         *[16]byte
	Cmdline      *[512]byte
	Id           *[32]byte
	ExtraCmdline *[1024]byte
}

func (d *DynImgV0) HdrSize() int {
	return binary.Size(BootImgHdrV0{})
}

func (d *DynImgV0) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}
	// copy data
	d.V2Hdr.BootImgHdrV0 = d.Raw

	d.DynImgCommon.Init()
	d.ExtraSize = &d.V2Hdr.HeaderVersion
	d.OsVersion = &d.V2Hdr.OsVersion
	d.Name = &d.V2Hdr.Name
	d.Cmdline = &d.V2Hdr.Cmdline
	d.Id = &d.V2Hdr.Id
	d.ExtraCmdline = &d.V2Hdr.ExtraCmdline
}

func (d *DynImgV0) PageSize() uint32 {
	return d.V2Hdr.PageSize
}

type DynImgV1 struct {
	DynImgV0

	Raw BootImgHdrV1

	RecoveryDtboSize   *uint32
	RecoveryDtboOffset *uint64
	HeaderSize         *uint32
}

func (d *DynImgV1) HeaderVersion() uint32 {
	return d.V2Hdr.HeaderVersion
}

func (d *DynImgV1) ExtraSize() uint32 {
	return 0
}

func (d *DynImgV1) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}
	// init before
	d.DynImgV0.Init(data)

	// copy data
	d.V2Hdr.BootImgHdrV1 = d.Raw

	d.RecoveryDtboSize = &d.V2Hdr.RecoveryDtboSize
	d.RecoveryDtboOffset = &d.V2Hdr.RecoveryDtboOffset
	d.HeaderSize = &d.V2Hdr.HeaderSize
}

type DynImgV2 struct {
	DynImgV1

	Raw BootImgHdrV2

	DtbSize *uint32
}

func (d *DynImgV2) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}
	// init before
	d.DynImgV1.Init(data)

	// copy data
	d.V2Hdr = d.Raw

	d.DtbSize = &d.V2Hdr.DtbSize
}

type DynImgPxa struct {
	DynImgCommon

	Raw BootImgHdrPxa

	ExtraSize    *uint32
	Name         *[24]byte
	Cmdline      *[512]byte
	Id           *[32]byte
	ExtraCmdline *[1024]byte
}

func (d *DynImgPxa) PageSize() uint32 {
	return d.HdrPxa.PageSize
}

func (d *DynImgPxa) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}

	// copy data
	d.HdrPxa = d.Raw

	d.ExtraSize = &d.HdrPxa.ExtraSize
	d.Name = &d.HdrPxa.Name
	d.Cmdline = &d.HdrPxa.Cmdline
	d.Id = &d.HdrPxa.Id
	d.ExtraCmdline = &d.HdrPxa.ExtraCmdline
}

type DynImgV3 struct {
	DynImgHdrBoot

	Raw BootImgHdrV3

	KernelSize   *uint32
	RamdiskSize  *uint32
	OsVersion    *uint32
	HeaderSize   *uint32
	Cmdline      *[1536]byte
	ExtraCmdline *[1536 - BOOT_ARGS_SIZE]byte
}

func (d *DynImgV3) HeaderVersion() uint32 {
	return d.V4Hdr.HeaderVersion
}

func (d *DynImgV3) PageSize() uint32 {
	return 4096
}

func (d *DynImgV3) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}

	// copy data
	d.V4Hdr.BootImgHdrV3 = d.Raw

	d.KernelSize = &d.V4Hdr.HeaderSize
	d.RamdiskSize = &d.V4Hdr.RamdiskSize
	d.OsVersion = &d.V4Hdr.OsVersion
	d.HeaderSize = &d.V4Hdr.HeaderSize
	d.Cmdline = &d.V4Hdr.Cmdline
	d.ExtraCmdline = (*[1536 - BOOT_ARGS_SIZE]byte)(unsafe.Pointer(&d.V4Hdr.Cmdline[BOOT_ARGS_SIZE]))

}

type DynImgV4 struct {
	DynImgV3

	Raw BootImgHdrV4
}

func (d *DynImgV4) SignatureSize() uint32 {
	return d.V4Hdr.SignatureSize
}

func (d *DynImgV4) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}

	d.DynImgV3.Init(data)
	// copy data
	d.V4Hdr = d.Raw
}

type DynImgHdrVendor struct {
	DynImgHdr
}

func (d *DynImgHdrVendor) IsVendor() bool {
	return true
}

type DynImgVndV3 struct {
	DynImgHdrVendor

	RamdiskSize *uint32
	Cmdline     *[2048]byte
	Name        *[16]byte
	HeaderSize  *uint32
	DtbSize     *uint32

	Raw BootImgHdrVndV3

	ExtraCmdline *[1536 - BOOT_ARGS_SIZE]byte
}

func (d *DynImgVndV3) HeaderVersion() uint32 {
	return d.V4Vnd.HeaderVersion
}

func (d *DynImgVndV3) PageSize() uint32 {
	return d.V4Vnd.PageSize
}

func (d *DynImgVndV3) HdrSpace() uint64 {
	return align_to(d.HdrSize(), uint64(d.PageSize()))
}

func (d *DynImgVndV3) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}

	d.RamdiskSize = &d.Raw.RamdiskSize
	d.Cmdline = &d.Raw.Cmdline
	d.Name = &d.Raw.Name
	d.HeaderSize = &d.Raw.HeaderSize
	d.DtbSize = &d.Raw.DtbSize

	// copy data
	d.V4Vnd.BootImgHdrVndV3 = d.Raw
}

type DynImgVndV4 struct {
	DynImgVndV3

	Raw BootImgHdrVndV4

	BootconfigSize *uint32
}

func (d *DynImgVndV4) VendorRamdiskTableSize() uint32 {
	return d.V4Vnd.VendorRamdiskTableSize
}

func (d *DynImgVndV4) VendorRamdiskTableEntryNum() uint32 {
	return d.V4Vnd.VendorRamdiskTableEntryNum
}

func (d *DynImgVndV4) VendorRamdiskTableEntrySize() uint32 {
	return d.V4Vnd.VendorRamdiskTableEntrySize
}

func (d *DynImgVndV4) Init(data []byte) {
	if len(data) < binary.Size(d.Raw) {
		log.Fatalln("Error: Could not parse data size less than struct")
	}

	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &d.Raw)
	if err != nil {
		log.Fatalln(err)
	}

	d.DynImgVndV3.Init(data)

	d.BootconfigSize = &d.Raw.BootconfigSize
	// copy data
	d.V4Vnd = d.Raw
}

const (
	MTK_KERNEL bootFlag = iota
	MTK_RAMDISK
	CHROMEOS_FLAG
	DHTB_FLAG
	SEANDROID_FLAG
	LG_BUMP_FLAG
	SHA256_FLAG
	BLOB_FLAG
	NOOKHD_FLAG
	ACCLAIM_FLAG
	AMONET_FLAG
	AVB1_SIGNED_FLAG
	AVB_FLAG
	ZIMAGE_KERNEL
	BOOT_FLAGS_MAX
)

type bootFlag int

type BootImg struct {
	Map mmap.MMap

	Hdr DynImgHdr

	Flags bootFlag

	K_fmt format_t
	R_fmt format_t
	E_fmt format_t

	Payload []byte
	Tail    []byte

	K_hdr *MtkHdr
	R_hdr *MtkHdr

	Z_hdr *ZimageHdr

	ZInfo struct {
		HdrSz uint32
		Tail  []byte
	}

	AvbFooter *AvbFooter
	Vbmeta    *AvbVBMetaImageHeader

	Kernel             *[]byte
	Ramdisk            *[]byte
	Second             *[]byte
	Extra              *[]byte
	RecoveryDtbo       *[]byte
	Dtb                *[]byte
	Signature          *[]byte
	VendorRamdiskTable *[]byte
	Bootconfig         *[]byte

	KernelDtb []byte

	Ignore []byte
}

func (b BootImg) New(file string) {

}

func (b *BootImg) ParseImage(addr *mmap.MMap, t format_t) {

}

func (b *BootImg) CreateHdr(addr mmap.MMap, t format_t) {

}

func (b *BootImg) GetPayload() []byte {
	return b.Payload
}

func (b *BootImg) GetTail() []byte {
	return b.Tail
}

func (b *BootImg) Verify(cert string) bool {
	return false // not impl
}

func decompress(t format_t, fd *os.File, in mmap.MMap) {
	decoder := NewDecoder(t, bytes.NewReader(in))
	io.Copy(fd, decoder)
}

func dump(buf []byte, size int, filename string) {
	if size == 0 {
		return
	}
	fd, err := os.Create(filename)
	if err != nil {
		log.Fatalln(err)
	}
	defer fd.Close()
	io.CopyN(fd, bytes.NewReader(buf), int64(size))
}

type fdtHeader struct {
	Magis           uint32
	TotalSize       uint32
	OffDtStruct     uint32
	OffDtStrings    uint32
	OffMemRsvmap    uint32
	Version         uint32
	LastCompVersion uint32
	BootCpuidPhys   uint32
	SizeDtStrings   uint32
	SizeDtStruct    uint32
}

//type nodeHeader struct {
//	Tag  uint32
//	Name []byte
//}

func findDtbOffset(fmap mmap.MMap, sz uint32) int {
	fmap_idx := 0
	end := fmap_idx + int(sz)

	for curr := fmap_idx; curr < fmap_idx+int(sz); curr += 40 {
		curr = bytes.Index(fmap, []byte{0xd0, 0x0d, 0xfe, 0xed})
		if curr == -1 {
			return -1
		}
		fdt_hdr := fdtHeader{}
		binary.Read(bytes.NewReader(fmap[curr:]), binary.BigEndian, &fdt_hdr)

		totalsize := fdt_hdr.TotalSize
		if totalsize > uint32(end-curr) {
			continue
		}

		off_dt_struct := fdt_hdr.OffDtStruct
		if off_dt_struct > uint32(end-curr) {
			continue
		}

		var fdt_node_hdr_tag uint32
		binary.Read(bytes.NewReader(fmap[curr+int(off_dt_struct):]), binary.BigEndian, &fdt_node_hdr_tag)
		if fdt_node_hdr_tag != 0x1 {
			continue
		}
		return curr - fmap_idx
	}
	return -1
}

func checkFmtLg(fmap mmap.MMap, sz uint32) format_t {
	f := CheckFmt(fmap)

	reader := bytes.NewReader(fmap)

	if f == LZ4_LEGACY {
		var off int64 = 4 // seems skip lz4_legacy header bytes
		var block_sz uint32
		for {
			if off+4 < int64(sz) {
				reader.Seek(int64(off), io.SeekStart)
				binary.Read(reader, binary.LittleEndian, &block_sz)
				off += 4
				if off+int64(block_sz) > int64(sz) {
					return LZ4_LG
				}
				off += int64(block_sz)
			}
		}
	}
	return f
}

func SplitImageDtb(filename string, skip_decomp bool) int {
	file, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	fmap, err := mmap.Map(file, 0, mmap.RDONLY)
	if err != nil {
		file.Close()
		log.Fatalln(err)
	}
	defer fmap.Unmap()
	defer file.Close()

	st, _ := os.Stat(filename)
	img_sz := uint32(st.Size()) // i have not seen big file kernel + dtb
	if off := findDtbOffset(fmap, img_sz); off > 0 {
		f := checkFmtLg(fmap, uint32(img_sz))
		if !skip_decomp && COMPRESSED(f) {
			fd, err := os.Create(KERNEL_FILE)
			if err != nil {
				log.Fatalln(err)
			}
			decompress(f, fd, fmap[off:])
			fd.Close()
		} else {
			dump(fmap, off, KERNEL_FILE)
		}
		dump(fmap[off:], int(img_sz)-off, KER_DTB_FILE)
	} else {
		fmt.Fprintln(os.Stderr, "Cannot find DTB in", filename)
		return 1
	}

	return 0

}
