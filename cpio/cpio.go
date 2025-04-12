package magiskboot

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"magiskboot/stub"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"slices"

	"github.com/edsrzf/mmap-go"
)

// Define this to avoid missing in different platform
const (
	O_CLOEXEC = 0x10000
	O_CREAT   = 0x0200
	O_RDONLY  = 0x0000
	O_TRUNC   = 0x0400
	O_WRONLY  = 0x0001
	S_IFBLK   = 0060000
	S_IFCHR   = 0020000
	S_IFDIR   = 0040000
	S_IFLNK   = 0120000
	S_IFMT    = 0170000
	S_IFREG   = 0100000
)

type CpioCli struct {
	File     string
	Commonds []string
}

type CpioAction int

const (
	TEST CpioAction = iota
	RESTORE
	PATCH
	EXIST
	BACKUP
	REMOVE
	MOVE
	EXTRACT
	LINK
	ADD
	LIST
)

type CpioCommand struct {
	Action CpioAction
}

type Test struct {
}

type Restore struct{}

type Patch struct{}

type Exists struct {
	Path string
}

type Backup struct {
	Origin         string
	SkipDecompress bool
}

type Remove struct {
	Path      string
	Recursive bool
}

type Move struct {
	Paths []string
}

type MakeDir struct {
	Mode uint32
	Path string
	File string
}

type List struct {
	Path      string
	Recursive bool
}

func PrintCpioUsage() {
	fmt.Fprint(os.Stderr, `Usage: magiskboot cpio <incpio> [commands...]

Do cpio commands to <incpio> (modifications are done in-place).
Each command is a single argument; add quotes for each command.

Supported commands:
  exists ENTRY
    Return 0 if ENTRY exists, else return 1
  ls [-r] [PATH]
    List PATH ("/" by default); specify [-r] to list recursively
  rm [-r] ENTRY
    Remove ENTRY, specify [-r] to remove recursively
  mkdir MODE ENTRY
    Create directory ENTRY with permissions MODE
  ln TARGET ENTRY
    Create a symlink to TARGET with the name ENTRY
  mv SOURCE DEST
    Move SOURCE to DEST
  add MODE ENTRY INFILE
    Add INFILE as ENTRY with permissions MODE; replaces ENTRY if exists
  extract [ENTRY OUT]
    Extract ENTRY to OUT, or extract all entries to current directory
  test
    Test the cpio's status. Return values:
    0:stock    1:Magisk    2:unsupported
  patch
    Apply ramdisk patches
    Configure with env variables: KEEPVERITY KEEPFORCEENCRYPT
  backup ORIG [-n]
    Create ramdisk backups from ORIG, specify [-n] to skip compression
  restore
    Restore ramdisk from ramdisk backup stored within incpio
`)
}

type CpioHeader struct {
	Magic     [6]byte // 魔术数字 "070701" 或 "070702" 表示新ASCII格式
	Ino       [8]byte // i-node 号
	Mode      [8]byte // 文件模式和权限
	Uid       [8]byte // 用户ID
	Gid       [8]byte // 组ID
	Nlink     [8]byte // 链接数
	Mtime     [8]byte // 修改时间
	Filesize  [8]byte // 文件大小
	Devmajor  [8]byte // 主设备号
	Devminor  [8]byte // 次设备号
	Rdevmajor [8]byte // 主设备号(特殊文件)
	Rdevminor [8]byte // 次设备号(特殊文件)
	Namesize  [8]byte // 文件名长度
	Check     [8]byte // 校验和(通常为0表示不检查)
}

type Cpio struct {
	Entries map[string]*CpioEntry
	Keys    []string

	fd *os.File
	mm *mmap.MMap
}

type CpioEntry struct {
	Mode      uint32
	Uid       uint32
	Gid       uint32
	RDevMajor uint32
	RDevMinor uint32
	Data      []byte
}

func NewCpio() *Cpio {
	return &Cpio{
		Entries: make(map[string]*CpioEntry),
		Keys:    make([]string, 0),
	}
}

func x8u(x []byte) (uint32, error) {
	if len(x) != 8 {
		return 0, errors.New("bad cpio header")
	}

	s := string(x)
	ret, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, err
	}

	return uint32(ret), nil
}

func align_4(x uint64) uint64 {
	return (x + 3) &^ 3
}

func norm_path(p string) string { /// dirty impl
	return strings.TrimLeft(path.Clean(p), "/")
}

func (c *Cpio) LoadFromData(data []byte) error {
	//c = NewCpio()
	pos := uint64(0)

	for pos < uint64(len(data)) {
		hdr_sz := binary.Size(CpioHeader{})
		hdr := CpioHeader{}
		reader := bytes.NewReader(data[pos : pos+uint64(hdr_sz)])
		binary.Read(reader, binary.LittleEndian, &hdr)
		if !bytes.Equal(hdr.Magic[:], []byte("070701")) {
			return errors.New("invalid cpio magic")
		}
		pos += uint64(hdr_sz)
		name_sz, err := x8u(hdr.Namesize[:])
		if err != nil {
			return err
		}
		name := strings.TrimRight(string(data[pos:pos+uint64(name_sz)]), "\x00")
		pos += uint64(name_sz)
		pos = align_4(pos)
		if name == "." || name == ".." {
			continue
		}
		if name == "TRAILER!!!" {
			// 在剩余数据中查找下一个魔术数字 "070701"
			nextHeader := bytes.Index(data[pos:], []byte("070701"))
			if nextHeader == -1 {
				break // 没有找到更多头部，结束处理
			}
			pos += uint64(nextHeader)
			continue
		}
		file_sz, _ := x8u(hdr.Filesize[:])
		xx8u := func(x [8]byte) uint32 {
			u, _ := x8u(x[:])
			return u
		}
		c.Entries[name] = &CpioEntry{
			Mode:      xx8u(hdr.Mode),
			Uid:       xx8u(hdr.Uid),
			Gid:       xx8u(hdr.Gid),
			RDevMajor: xx8u(hdr.Rdevmajor),
			RDevMinor: xx8u(hdr.Rdevminor),
			Data:      data[pos : pos+uint64(file_sz)],
		}
		c.Keys = append(c.Keys, name)
		pos += uint64(file_sz)
		pos = align_4(pos)
	}
	return nil
}

func (c *Cpio) LoadFromFile(path string) error {
	fmt.Fprintf(os.Stderr, "Loading cpio: [%s]\n", path)
	fd, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	c.fd = fd
	m, err := mmap.Map(fd, 0, mmap.RDWR)
	if err != nil {
		fd.Close()
		return err
	}
	c.mm = &m
	// It looks we has been loaded all file into cpio struture...
	// Do not forget to close all these
	c.LoadFromData(m)

	// At Close()
	//defer m.Flush()
	//defer m.Unmap()
	//defer fd.Close()
	return nil
}

func (c *Cpio) Close() {
	c.mm.Flush()
	c.mm.Unmap()
	c.fd.Close()
}

func writeZeros(fd io.Writer, pos uint64) uint64 {
	buf := make([]byte, align_4(pos)-pos)
	write_len, err := fd.Write(buf)
	if err != nil {
		log.Fatalln(err)
	}
	return uint64(write_len)
}

// It seems create a cpio file
func (c *Cpio) Dump(path string) error {
	fmt.Fprintf(os.Stderr, "Dumping cpio [%s]\n", path)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	pos := uint64(0)
	inode := int64(300000)
	for _, name := range c.Keys {
		entry := c.Entries[name]
		header := fmt.Sprintf(
			"070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
			inode,
			entry.Mode,
			entry.Uid,
			entry.Gid,
			1, // nlink
			0, // mtime
			len(entry.Data),
			0, // major
			0, // minor
			entry.RDevMajor,
			entry.RDevMinor,
			len(name)+1, // namesize (including null terminator)
			0,           // chksum
		)
		write_len, err := file.Write([]byte(header))
		if err != nil {
			defer os.Remove(path)
			return err
		}
		pos += uint64(write_len)
		write_len, _ = file.Write([]byte(name))
		pos += uint64(write_len)
		write_len, _ = file.Write([]byte{0})
		pos += uint64(write_len)
		pos += writeZeros(file, pos)
		pos = align_4(pos)
		write_len, _ = file.Write(entry.Data)
		pos += uint64(write_len)
		writeZeros(file, pos)
		pos = align_4(pos)
		inode += 1
	}
	header := fmt.Sprintf("070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x", inode, 0o755, 0, 0, 1, 0, 0, 0, 0, 0, 0, 11, 0)
	write_len, _ := file.Write([]byte(header))
	pos += uint64(write_len)
	write_len, _ = file.Write([]byte("TRAILER!!!\x00"))
	pos += uint64(write_len)
	writeZeros(file, pos)

	return nil
}

func (c *Cpio) Rm(path string, recursive bool) {
	path = norm_path(path)
	removeByValue := func(slice []string, value string) []string {
		for i, v := range slice {
			if v == value {
				return append(slice[:i], slice[i+1:]...)
			}
		}
		return slice
	}
	removeEntry := func(k string) bool {
		delete(c.Entries, k)
		c.Keys = removeByValue(c.Keys, k)
		if _, exists := c.Entries[k]; !exists {
			return true
		}
		return false
	}
	if _, exist := c.Entries[path]; exist {
		if removeEntry(path) {
			fmt.Fprintf(os.Stderr, "Removed entry [%s]\n", path)
		}
	}
	if recursive {
		path = path + "/"
		for k := range c.Entries {
			if strings.HasPrefix(k, path) {
				if removeEntry(k) {
					fmt.Fprintf(os.Stderr, "Removed entry [%s]\n", k)
				}
			}
		}
	}
}

func (c *Cpio) extractEntry(p, out string) error {
	if !slices.Contains(c.Keys, p) {
		log.Fatalln("No such file")
	}

	entry := c.Entries[p]
	fmt.Fprintf(os.Stderr, "Extracting entry [%s] to [%s]\n", p, out)

	_, err := os.Stat(path.Dir(p))
	if os.IsNotExist(err) {
		os.MkdirAll(path.Dir(p), 0o755)
	}

	mode := os.FileMode(entry.Mode & 0o777)

	switch entry.Mode & S_IFMT {
	case S_IFDIR:
		return os.Mkdir(out, mode)
	case S_IFREG:
		file, err := os.Create(out)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.Write(entry.Data)
		return err
	case S_IFLNK:
		lnk := string(bytes.ReplaceAll(entry.Data, []byte{0}, []byte{}))
		return os.Symlink(lnk, out)
	case S_IFBLK | S_IFCHR:
		if runtime.GOOS != "windows" {
			dev := stub.Mkdev(entry.RDevMajor, entry.RDevMinor)
			return stub.Mknod(out, uint32(mode), int(dev))
		} else {
			return nil
		}
	default:
		return errors.New("unknow entry type")
	}
}

// if *p and *out is nil, extract all entries in current dir
func (c *Cpio) Extract(p, out *string) error {
	if p != nil && out != nil {
		path := norm_path(*p)
		return c.extractEntry(path, *out)
	} else {
		for _, path := range c.Keys {
			if path == "." || path == ".." {
				continue
			}
			if err := c.extractEntry(path, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Cpio) Exists(path string) bool {
	return slices.Contains(c.Keys, path)
}

func (c *Cpio) addEntry(key string, entry *CpioEntry) {
	c.Entries[key] = entry
	c.Keys = append(c.Keys, key)
	// Sort c.Keys like rust BTreeMap
	sort.Strings(c.Keys)
}

func (c *Cpio) Add(mode uint32, path string, file string) error {
	if strings.HasSuffix(path, "/") {
		return errors.New("path cannot end with / for add")
	}

	attr, err := os.Stat(file)
	if err != nil {
		return err
	}
	var content []byte
	rdevmajor := uint64(0)
	rdevminor := uint64(0)

	mode = func() uint32 {
		if attr.Mode().IsRegular() || (attr.Mode()&os.ModeSymlink != 0) {
			rdevmajor = 0
			rdevminor = 0
			content, err = os.ReadFile(file)
			if err != nil {
				log.Fatalln(err)
			}
			return mode | S_IFREG
		} else {
			if runtime.GOOS != "windows" {
				uattr := stub.Stat_t{}
				err = stub.Stat(file, &uattr)
				//uattr := &attr
				if err != nil {
					log.Fatalln(err)
				}
				rdevmajor = uint64(stub.Major(uint64(uattr.Rdev)))
				rdevminor = uint64(stub.Minor(uint64(uattr.Rdev)))
				if attr.Mode()&os.ModeDevice != 0 {
					mode = mode | S_IFBLK
				} else if attr.Mode()&os.ModeCharDevice != 0 {
					mode = mode | S_IFCHR
				} else {
					log.Fatalln("unsupport file type")
				}
			}
		}
		return mode
	}()

	c.addEntry(norm_path(path), &CpioEntry{
		Mode:      mode,
		Uid:       0,
		Gid:       0,
		RDevMajor: uint32(rdevmajor),
		RDevMinor: uint32(rdevminor),
		Data:      content,
	})
	fmt.Fprintf(os.Stderr, "Add file [%s] (%04o)\n", path, mode)
	return nil
}

func (c *Cpio) Mkdir(mode uint32, dir string) {
	c.addEntry(norm_path(dir), &CpioEntry{
		Mode:      mode | S_IFDIR,
		Uid:       0,
		Gid:       0,
		RDevMajor: 0,
		RDevMinor: 0,
		Data:      []byte{},
	})
	fmt.Fprintf(os.Stderr, "Create directory [%s] (%04o)\n", dir, mode)
}

func (c *Cpio) Ln(src, dst string) {
	c.addEntry(norm_path(dst), &CpioEntry{
		Mode:      S_IFLNK,
		Uid:       0,
		Gid:       0,
		RDevMajor: 0,
		RDevMinor: 0,
		Data: func() []byte {
			ret := norm_path(src)
			if strings.HasPrefix(src, "/") {
				ret = "/" + ret
			}
			return []byte(ret)
		}(),
	})
	fmt.Fprintf(os.Stderr, "Create symlink [%s] -> [%s]\n", dst, src)
}
