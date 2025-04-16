package magiskboot

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsnet/compress/bzip2"
	"github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

const CHUNK int = 0x40000
const LZ4_UNCOMPRESSED int = 0x800000

var LZ4_COMPRESSED int = lz4.CompressBlockBound(LZ4_UNCOMPRESSED)

type Encoder struct {
	writeCloser io.WriteCloser
}

type Lz4HCWriter struct {
	*lz4.CompressorHC

	writer io.Writer
	lg     bool

	buf []byte

	in_total uint32
}

func NewLz4HCWriter(writer io.Writer, lg bool) *Lz4HCWriter {
	z := new(Lz4HCWriter)
	z.CompressorHC = &lz4.CompressorHC{
		Level: lz4.Level9,
	}
	z.writer = writer
	z.lg = lg
	z.buf = make([]byte, LZ4_COMPRESSED)

	writer.Write([]byte{0x02, 0x21, 0x4c, 0x18})

	return z
}

func (z *Lz4HCWriter) Write(data []byte) (int, error) {
	var write_len = 0
	var block_sz uint32

	sz, err := z.CompressBlock(data, z.buf)
	if err != nil {
		log.Fatalln(err)
	}

	block_sz = uint32(sz)

	if block_sz == 0 {
		log.Fatalln("LZ4HC compression failure")
	}
	binary.Write(z.writer, binary.LittleEndian, &block_sz)
	write_len += binary.Size(block_sz)

	l, err := z.writer.Write(z.buf[:block_sz])
	if err != nil {
		log.Fatalln(err)
	}
	write_len += l
	z.in_total += uint32(len(data))

	return write_len, err
}

func (z *Lz4HCWriter) Close() error {
	if z.lg {
		return binary.Write(z.writer, binary.LittleEndian, &z.in_total)
	}
	return nil
}

func NewEncoder(t format_t, writer io.Writer) *Encoder {
	encoder := new(Encoder)
	var w io.WriteCloser = nil
	var err error = nil

	switch t {
	case XZ:
		w, err = xz.NewWriter(writer)
	case LZMA:
		w, err = lzma.NewWriter(writer)
	case BZIP2:
		w, err = bzip2.NewWriter(writer, &bzip2.WriterConfig{
			Level: 9,
		})
	case LZ4:
		w = lz4.NewWriter(writer)
		w.(*lz4.Writer).Apply(
			lz4.BlockChecksumOption(false),
			lz4.BlockSizeOption(lz4.Block4Mb),
			lz4.CompressionLevelOption(lz4.Level9),
			lz4.ChecksumOption(true))
	case LZ4_LEGACY:
		w = NewLz4HCWriter(writer, false)
	case LZ4_LG:
		w = NewLz4HCWriter(writer, true)
	case ZOPFLI: // not support
		panic("compress: not impl zopfli on this magiskboot")
	case GZIP:
		w = gzip.NewWriter(writer)
	}

	if err != nil {
		log.Fatalln(err)
	}

	encoder.writeCloser = w

	return encoder
}

func (e *Encoder) Write(data []byte) (int, error) {
	return e.writeCloser.Write(data)
}

func (e *Encoder) Close() error {
	return e.writeCloser.Close()
}

type Decoder struct {
	reader io.Reader
	closer io.Closer
}

func NewDecoder(t format_t, reader io.Reader) *Decoder {
	decoder := new(Decoder)
	var r io.Reader = nil
	var err error = nil

	decoder.reader = nil
	decoder.closer = nil

	switch t {
	case XZ:
		r, err = xz.NewReader(reader)
	case LZMA:
		r, err = lzma.NewReader(reader)
	case BZIP2:
		r, err = bzip2.NewReader(reader, &bzip2.ReaderConfig{})
	case LZ4:
		r = lz4.NewReader(reader)
	case LZ4_LEGACY, LZ4_LG:
		r = lz4.NewReader(reader)
	case ZOPFLI, GZIP:
		r, err = gzip.NewReader(reader)
		if err == nil {
			decoder.closer = r.(io.Closer)
		}
	}
	if err != nil {
		log.Fatalln(err)
	}
	decoder.reader = r
	return decoder
}

func (d *Decoder) Decode() ([]byte, error) {
	if d.reader == nil {
		return nil, errors.New("decoder not initialized")
	}
	return io.ReadAll(d.reader)
}

func (d *Decoder) Read(data []byte) (int, error) {
	return d.reader.Read(data)
}

func (d *Decoder) Close() error {
	if d.closer != nil {
		return d.closer.Close()
	}
	return nil
}

func Compress(method, infile, outfile string) {
	t := Name2Fmt(method)
	if t == UNKNOWN {
		log.Fatalln("Unknow compression method: ", method)
	}

	in_std := infile == "-"
	rm_in := false

	in_fd := func() *os.File {
		if in_std {
			return os.Stdin
		} else {
			file, err := os.Open(infile)
			if err != nil {
				log.Fatalln(err)
			}
			return file
		}
	}()

	var out_fd *os.File = nil
	if outfile == "" {
		if in_std {
			out_fd = os.Stdout
		} else {
			var err error = nil
			tmp := infile + Fmt2Ext(t)
			out_fd, err = os.Create(tmp)
			if err != nil {
				log.Fatalln(err)
			}
			fmt.Fprintf(os.Stderr, "Compressing to [%s]\n", tmp)
			rm_in = true
		}
	} else {
		out_fd = func() *os.File {
			if outfile == "-" {
				return os.Stdout
			} else {
				file, err := os.Create(outfile)
				if err != nil {
					log.Fatalln(err)
				}
				return file
			}
		}()
	}

	encoder := NewEncoder(t, out_fd)

	buf := make([]byte, 4096)
	for {
		l, err := in_fd.Read(buf)

		if _, err := encoder.Write(buf[:l]); err != nil {
			log.Fatalln(err)
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalln(err)
		}
	}
	encoder.Close()

	if in_fd != os.Stdin {
		in_fd.Close()
	}

	if out_fd != os.Stdout {
		out_fd.Close()
	}

	if rm_in {
		os.Remove(infile)
	}
}

func Decompress(infile, outfile string) {
	in_std := infile == "-"
	rm_in := false

	in_fd := func() *os.File {
		if in_std {
			return os.Stdin
		}
		file, err := os.Open(infile)
		if err != nil {
			log.Fatalln(err)
		}
		return file
	}()
	//defer in_fd.Close()

	buf := make([]byte, 4096)
	_, err := in_fd.Read(buf)
	if err != nil {
		log.Fatalln(err)
	}
	in_fd.Seek(0, io.SeekStart)

	t := CheckFmt(buf)
	if !COMPRESSED(t) {
		log.Fatalln("Input file is not a supported compressed type!")
	}

	if outfile == "" {
		outfile = infile
		if !in_std {
			ext := filepath.Ext(infile)
			if ext != "" {
				if ext != Fmt2Ext(t) {
					log.Fatalln("Input file is not a supported type!")
				}

				outfile = strings.TrimRight(infile, ext)
				rm_in = true
				fmt.Fprintf(os.Stderr, "Decompressing to [%s]\n", outfile)
			}
		}
	}

	out_fd := func() *os.File {
		if outfile == "-" {
			return os.Stdout
		}
		file, err := os.Create(outfile)
		if err != nil {
			log.Fatalln(err)
		}
		return file
	}()

	decoder := NewDecoder(t, in_fd)
	defer decoder.Close()
	/*
		decompressed, err := decoder.Decode()
		if err != nil {
			log.Fatalln("Decompression error\n", err)
		}

		_, err = out_fd.Write(decompressed)
		if err != nil {
			log.Fatalln(err)
		}
	*/
	//buf = make([]byte, 4096)
	for {
		// 读取数据
		_len, err := decoder.Read(buf)
		if _len > 0 {
			// 写入读取到的数据
			_, writeErr := out_fd.Write(buf[:_len])
			if writeErr != nil {
				log.Fatalln("Write error:", writeErr)
			}
		}

		// 处理错误
		if err != nil {
			if err == io.EOF {
				break // 正常结束
			}
			log.Fatalln("Read error:", err)
		}
	}

	if in_fd != os.Stdin {
		in_fd.Close()
	}
	if out_fd != os.Stdout {
		out_fd.Close()
	}

	if rm_in {
		os.Remove(infile)
	}
}

func DecompressToFd(data []byte, fd *os.File) bool {
	t := CheckFmt(data)

	if !COMPRESSED(t) {
		log.Println("Input file is not a supported compression format!")
		return false
	}

	decoder := NewDecoder(t, bytes.NewReader(data))
	d, err := decoder.Decode()
	if err != nil {
		log.Fatalln(err)
	}

	_, err = fd.Write(d)
	if err != nil {
		log.Fatalln(err)
	}
	return true
}

func Xz(data []byte, compressed *[]byte) bool {
	bufferc := new(bytes.Buffer)
	xzwriter, err := xz.NewWriter(bufferc)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	defer xzwriter.Close()
	_, err = xzwriter.Write(data)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	*compressed = bufferc.Bytes()
	return true
}

func Unxz(data []byte, decompressed *[]byte) bool {
	t := CheckFmt(data)
	if t != XZ {
		log.Println("Input file is not in xz format!")
		return false
	}
	buffer := bytes.NewBuffer(data)
	xzreader, err := xz.NewReader(buffer)
	if err != nil {
		log.Println("Error:", err)
		return false
	}

	d, err := io.ReadAll(xzreader)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	*decompressed = d
	return true
}
