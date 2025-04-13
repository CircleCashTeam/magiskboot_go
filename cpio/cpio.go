package magiskboot

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"magiskboot"
	"magiskboot/stub"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"slices"

	"github.com/dustin/go-humanize"
	"github.com/edsrzf/mmap-go"
	"github.com/ulikunitz/xz"
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

const (
	S_IRUSR = 0400
	S_IWUSR = 0200
	S_IXUSR = 0100

	S_IRGRP = 0040
	S_IWGRP = 0020
	S_IXGRP = 0010

	S_IROTH = 0004
	S_IWOTH = 0002
	S_IXOTH = 0001
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
	Entries map[string]CpioEntry
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
		Entries: make(map[string]CpioEntry),
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
		c.Entries[name] = CpioEntry{
			Mode:      xx8u(hdr.Mode),
			Uid:       xx8u(hdr.Uid),
			Gid:       xx8u(hdr.Gid),
			RDevMajor: xx8u(hdr.Rdevmajor),
			RDevMinor: xx8u(hdr.Rdevminor),
			Data:      bytes.Clone(data[pos : pos+uint64(file_sz)]),
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

	// When reading done, close
	c.Close()
	return nil
}

func (c *Cpio) Close() {
	// This may cause c.Dump failed
	//c.mm.Flush() // Do not flush memories
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

func (c *Cpio) addEntry(key string, entry CpioEntry) {
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

	c.addEntry(norm_path(path), CpioEntry{
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
	c.addEntry(norm_path(dir), CpioEntry{
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
	c.addEntry(norm_path(dst), CpioEntry{
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

func (c *Cpio) Mv(from, to string) error {
	from = norm_path(from)
	to = norm_path(to)
	entry := c.Entries[from]
	newk := make([]string, 0)
	for _, k := range c.Keys {
		if k != from {
			newk = append(newk, k)
		}
	}
	delete(c.Entries, from)
	c.Keys = newk
	c.addEntry(to, entry)
	fmt.Fprintf(os.Stderr, "Move [%s] -> [%s]\n", from, to)
	return nil
}

func (c *Cpio) Ls(path string, recursive bool) {
	path = norm_path(path)
	if path != "" {
		path = "/" + path
	}

	for _, name := range c.Keys {
		entry := c.Entries[name]
		p := "/" + name
		if !strings.HasPrefix(p, path) {
			continue
		}
		p = strings.TrimPrefix(p, path)
		if p != "" && !strings.HasPrefix(p, "/") {
			continue
		}
		if !recursive && p != "" && strings.Count(p, "/") > 1 {
			continue
		}
		//fmt.Printf("%s\n", name)
		fmt.Fprintf(os.Stdout, "%v\t%s\n", entry, name)
	}
}

// Make cpio.ls print formatable
//
// Example:
//
//	fmt.Printf("%v\n", e)
func (entry CpioEntry) Format(f fmt.State, verb rune) {
	io.WriteString(f, fmt.Sprintf("%8s%8d%8d%8s%4d:%-8d",
		func() string {
			var a, b, c, d, e, f, g, h, i, j byte
			switch entry.Mode & S_IFMT {
			case S_IFDIR:
				a = 'd'
			case S_IFREG:
				a = '-'
			case S_IFLNK:
				a = 'l'
			case S_IFBLK:
				a = 'b'
			case S_IFCHR:
				a = 'c'
			default:
				a = '?'
			}
			b = '-'
			if entry.Mode&S_IRUSR != 0 {
				b = 'r'
			}
			c = '-'
			if entry.Mode&S_IWUSR != 0 {
				c = 'w'
			}
			d = '-'
			if entry.Mode&S_IXUSR != 0 {
				d = 'x'
			}
			e = '-'
			if entry.Mode&S_IRGRP != 0 {
				e = 'r'
			}
			f = '-'
			if entry.Mode&S_IWGRP != 0 {
				f = 'w'
			}
			g = '-'
			if entry.Mode&S_IXGRP != 0 {
				g = 'x'
			}
			h = '-'
			if entry.Mode&S_IROTH != 0 {
				h = 'r'
			}
			i = '-'
			if entry.Mode&S_IWOTH != 0 {
				i = 'w'
			}
			j = '-'
			if entry.Mode&S_IXOTH != 0 {
				j = 'x'
			}
			return fmt.Sprintf("%c%c%c%c%c%c%c%c%c%c", a, b, c, d, e, f, g, h, i, j)
		}(),
		entry.Uid,
		entry.Gid,
		humanize.Bytes(uint64(len(entry.Data))),
		entry.RDevMajor,
		entry.RDevMinor,
	))
}

func _xz(data, compressed *[]byte) bool {
	bufferc := bytes.NewBuffer(*compressed)
	xzwriter, err := xz.NewWriter(bufferc)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	_, err = xzwriter.Write(*data)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	return true
}

func (entry *CpioEntry) Compress() bool {
	if entry.Mode&S_IFMT != S_IFREG {
		return false
	}
	compressed := make([]byte, 0)
	if !_xz(&entry.Data, &compressed) {
		fmt.Fprintln(os.Stderr, "xz compression failed")
		return false
	}
	entry.Data = compressed
	return true
}

func _unxz(data, decompressed *[]byte) bool {
	buffer := bytes.NewBuffer(*data)
	xzreader, err := xz.NewReader(buffer)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	_, err = xzreader.Read(*decompressed)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	return true
}

func (entry *CpioEntry) Decompress() bool {
	if entry.Mode&S_IFMT != S_IFREG {
		return false
	}
	decompressed := make([]byte, 0)
	if !_unxz(&entry.Data, &decompressed) {
		fmt.Fprintln(os.Stderr, "xz decompression failed")
		return false
	}
	entry.Data = decompressed
	return true
}

const MAGISK_PATCHED int32 = 1 << 0
const UNSUPPORTED_CPIO int32 = 1 << 1

func (c *Cpio) Patch() {
	keep_verity := magiskboot.CheckEnv("KEEPVERITY")
	keep_force_encrypt := magiskboot.CheckEnv("KEEPFORCEENCRYPT")
	fmt.Fprintf(os.Stderr, "Patch with flag KEEPVERITY=[%v] KEEPFORCEENCRYPT=[%v]\n",
		keep_verity, keep_force_encrypt,
	)
	for _, name := range c.Keys {
		entry := c.Entries[name]
		fstab := func() bool {
			return (!keep_verity || !keep_force_encrypt) &&
				entry.Mode&S_IFMT == S_IFREG &&
				!strings.HasPrefix(name, ".backup") &&
				!strings.HasPrefix(name, "twrp") &&
				!strings.HasPrefix(name, "recovery") &&
				strings.HasPrefix(name, "fstab")
		}()
		if !keep_verity {
			if fstab {
				fmt.Fprintf(os.Stderr, "Found fstab file [%s]\n", name)
				entry.Data = magiskboot.PatchVerity(entry.Data)
			} else if name == "verity_key" {
				c.Rm(name, false)
			}
		}
		if !keep_force_encrypt && fstab {
			entry.Data = magiskboot.PatchEncryption(entry.Data)
		}
	}
}

func (c *Cpio) Test() int32 {
	for _, file := range []string{
		"sbin/launch_daemonsu.sh",
		"sbin/su",
		"init.xposed.rc",
		"boot/sbin/launch_daemonsu.sh",
	} {
		if slices.Contains(c.Keys, file) {
			return UNSUPPORTED_CPIO
		}
	}
	for _, file := range []string{
		".backup/.magisk",
		"init.magisk.rc",
		"overlay/init.magisk.rc",
	} {
		if slices.Contains(c.Keys, file) {
			return MAGISK_PATCHED
		}
	}
	return 0
}

func (c *Cpio) Restore() error {
	backups := make(map[string]CpioEntry, 0)
	var rm_list strings.Builder

	for _, name := range c.Keys {
		entry := c.Entries[name]
		if strings.HasPrefix(name, ".backup/") {
			if name == ".backup/.rmlist" {
				//rm_list = c.Entries[name].Data
				_, err := rm_list.Write(c.Entries[name].Data)
				if err != nil {
					return err
				}
			} else if name != ".backup/.magisk" {
				new_name := func() string {
					if strings.HasSuffix(name, ".xz") && entry.Decompress() {
						return name[8 : len(name)-3]
					} else {
						return name[8:]
					}
				}()
				backups[new_name] = entry
			}
		}
	}
	c.Rm(".backup", false)
	if rm_list.Len() == 0 && len(backups) == 0 {
		for k := range c.Entries {
			delete(c.Entries, k)
		}
		return nil
	}

	for _, rm := range strings.Split(rm_list.String(), "\x00") {
		if len(rm) != 0 {
			c.Rm(rm, false)
		}
	}
	for k, v := range backups {
		c.Keys = append(c.Keys, k)
		c.Entries[k] = v
	}
	slices.Sort(c.Keys)
	return nil
}

func (c *Cpio) Backup(origin string, skip_compress bool) error {
	backups := make(map[string]CpioEntry)
	var rm_list strings.Builder

	backups[".backup"] = CpioEntry{
		Mode:      S_IFDIR,
		Uid:       0,
		Gid:       0,
		RDevMajor: 0,
		RDevMinor: 0,
		Data:      []byte{},
	}
	o := NewCpio()
	o.LoadFromFile(origin)
	o.Close()

	o.Rm(".backup", true)
	c.Rm(".backup", true)

	lhs := o.Entries
	rhs := c.Entries
	lhsKeys := o.Keys // 建议使用驼峰命名
	rhsKeys := c.Keys

	lhsIndex, rhsIndex := 0, 0

	backupFunc := func(name string, entry CpioEntry) {
		backupPath := ".backup/" + name
		if !skip_compress && entry.Compress() {
			backupPath += ".xz"
		}
		fmt.Fprintf(os.Stderr, "Backup [%s] -> [%s]\n", name, backupPath)
		// 需要实际将entry添加到backups map中
		backups[name] = entry
	}

	recordFunc := func(name string) {
		fmt.Fprintf(os.Stderr, "Record new entry [%s] -> [.backup/.rmlist]\n", name)
		rm_list.WriteString(name)
		rm_list.WriteByte('\x00')
	}

	for lhsIndex < len(lhsKeys) && rhsIndex < len(rhsKeys) {
		lKey := lhsKeys[lhsIndex]
		rKey := rhsKeys[rhsIndex]
		re := rhs[rKey] // 先获取当前右侧条目

		switch cmp.Compare(lKey, rKey) {
		case -1: // lhs < rhs
			le := lhs[lKey]
			backupFunc(lKey, le)
			lhsIndex++
		case 0: // lhs == rhs
			le := lhs[lKey]
			if !bytes.Equal(re.Data, le.Data) {
				backupFunc(lKey, le)
			}
			lhsIndex++
			rhsIndex++
		case 1: // lhs > rhs
			recordFunc(rKey)
			rhsIndex++
		}
	}

	// 处理剩余元素
	for ; lhsIndex < len(lhsKeys); lhsIndex++ {
		lKey := lhsKeys[lhsIndex]
		le := lhs[lKey]
		backupFunc(lKey, le)
	}

	for ; rhsIndex < len(rhsKeys); rhsIndex++ {
		rKey := rhsKeys[rhsIndex]
		recordFunc(rKey)
	}

	if rm_list.Len() != 0 {
		backups[".backup/.rmlist"] = CpioEntry{
			Mode:      S_IFREG,
			Uid:       0,
			Gid:       0,
			RDevMajor: 0,
			RDevMinor: 0,
			Data:      []byte(rm_list.String()),
		}
	}

	for k, v := range backups {
		c.Keys = append(c.Keys, k)
		c.Entries[k] = v
	}
	slices.Sort(c.Keys)

	return nil
}

func NewCpioCli() *CpioCli {
	return new(CpioCli)
}

func (c *CpioCli) FromArgs(args []string) {
	c.File = args[0]
	c.Commonds = args[1:]
}

func parseMode(mode string) uint32 {
	ret, err := strconv.ParseInt(mode, 8, 32)
	if err != nil {
		log.Fatalln("Error:", err)
	}
	return uint32(ret)
}

func CpioCommands(argv []string) {
	if len(argv) < 1 {
		log.Fatalln("No arguments")
	}

	cli := NewCpioCli()
	cli.FromArgs(argv)
	cpio := NewCpio()

	if _, err := os.Stat(cli.File); err == nil {
		if err := cpio.LoadFromFile(cli.File); err != nil {
			log.Fatalf("加载cpio文件失败: %v", err)
		}
	}

	errExit := func() {
		PrintCpioUsage()
		os.Exit(125)
	}

	for _, command := range cli.Commonds {
		if strings.HasPrefix(command, "#") {
			continue
		}
		cmd := strings.Split(command, " ")
		switch cmd[0] {
		case "test":
			os.Exit(int(cpio.Test()))
		case "restore":
			cpio.Restore()
		case "patch":
			cpio.Patch()
		case "exists":
			if len(cmd) > 1 {
				if cpio.Exists(cmd[1]) {
					os.Exit(0)
				} else {
					os.Exit(1)
				}
			} else {
				errExit()
			}
		case "backup":
			if len(cmd) > 1 {
				skip_compress := false
				if len(cmd) > 2 {
					if cmd[2] == "-n" {
						skip_compress = true
					}
				}
				if err := cpio.Backup(cmd[1], skip_compress); err != nil {
					log.Fatalln(err)
				}
			} else {
				errExit()
			}
		case "rm":
			if len(cmd) > 1 {
				recursive := false
				path := cmd[1]
				if cmd[1] == "-r" {
					recursive = true
					path = cmd[2]
				}
				cpio.Rm(path, recursive)
			}
		case "mv":
			if len(cmd) > 2 {
				from := cmd[1]
				to := cmd[2]
				cpio.Mv(from, to)
			} else {
				errExit()
			}
		case "ln":
			if len(cmd) > 2 {
				src, dst := cmd[1], cmd[2]
				cpio.Ln(src, dst)
			} else {
				errExit()
			}
		case "mkdir":
			if len(cmd) > 2 {
				mode, dir := parseMode(cmd[1]), cmd[2]
				cpio.Mkdir(mode, dir)
			} else {
				errExit()
			}
		case "add":
			if len(cmd) > 3 {
				mode, path, file := parseMode(cmd[1]), cmd[2], cmd[3]
				if err := cpio.Add(mode, path, file); err != nil {
					log.Fatalln(err)
				}
			} else {
				errExit()
			}
		case "extract":
			if len(cmd) > 1 {
				var path, out *string = &cmd[1], nil
				if len(cmd) > 2 {
					out = &cmd[2]
				}
				if err := cpio.Extract(path, out); err != nil {
					log.Fatalln(err)
				}
			}
		case "ls":
			if len(cmd) == 1 {
				cpio.Ls("/", true)
			} else if len(cmd) == 2 {
				path := cmd[1]
				cpio.Ls(path, false)
			} else if len(cmd) > 2 {
				recursive := false
				path := cmd[2]
				if cmd[1] == "-r" {
					recursive = true
				}
				cpio.Ls(path, recursive)
			} else {
				errExit()
			}
			os.Exit(0)
		}
	}
	err := cpio.Dump(cli.File)
	if err != nil {
		log.Fatalln(err)
	}
}
