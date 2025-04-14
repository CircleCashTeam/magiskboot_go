package magiskboot

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"

	"github.com/ulikunitz/xz"
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
	Fmt   format_t
	Outfd *os.File
}

func NewDecoder(t format_t, file *os.File) *Decoder {
	return &Decoder{
		Fmt:   t,
		Outfd: file,
	}
}

func (*Decoder) Read(reader io.Reader) ([]byte, error) {
	return nil, errors.New("todo: not impl yet")
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
