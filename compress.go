package magiskboot

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

type Encoder struct {
	Fmt   format_t
	Outfd *os.File
}

func NewEncoder(t format_t, file *os.File) *Encoder {
	return &Encoder{
		Fmt:   t,
		Outfd: file,
	}
}

func (*Encoder) Write(data []byte, writer io.Writer) (int64, error) {
	return 0, errors.New("todo: not impl yet")
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
		r = bzip2.NewReader(reader)
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
