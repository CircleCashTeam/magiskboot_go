package magiskboot

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/edsrzf/mmap-go"
)

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\t' || b == '\r'
}

func removePatterns(data []byte, patterns [][]byte) ([]byte, int) {
	var result []byte
	originalLen := len(data)
	removed := 0

	for i := 0; i < len(data); {
		matched := false

		for _, pat := range patterns {
			if bytes.HasPrefix(data[i:], pat) {
				fmt.Fprintf(os.Stderr, "Remove pattern [%s]\n", pat)
				end := i + len(pat)
				for end < len(data) && !isWhitespace(data[end]) {
					if data[end] == ',' {
						end++
						break
					}
					end++
				}
				removed += end - i
				i = end
				matched = true
				break
			}
		}

		if !matched {
			result = append(result, data[i])
			i++
		}
	}

	return result, originalLen - removed
}

func PatchVerify(data []byte) []byte {
	patterns := [][]byte{
		[]byte("verifyatboot"),
		[]byte("verify"),
		[]byte("avb_keys"),
		[]byte("avb"),
		[]byte("support_scfs"),
		[]byte("fsverity"),
	}
	newdata, _ := removePatterns(data, patterns)
	return newdata
}

func PatchEncryption(data []byte) []byte {
	patterns := [][]byte{
		[]byte("forceencrypt"),
		[]byte("forcefdeorfbe"),
		[]byte("fileencryption"),
	}
	newdata, _ := removePatterns(data, patterns)
	return newdata
}

func HexPatch(file, from, to string) bool {
	fd, err := os.OpenFile(file, os.O_RDWR, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	defer fd.Close()
	fstat, err := fd.Stat()
	if err != nil {
		log.Fatalln(err)
	}
	fsize := fstat.Size()

	from_b, err := hex.DecodeString(from)
	if err != nil {
		log.Fatalln(err)
	}
	to_b, err := hex.DecodeString(to)
	if err != nil {
		log.Fatalln(err)
	}

	patched := false
	if m, err := mmap.Map(fd, mmap.RDWR, 0); err == nil {
		for i := int64(0); i < fsize; i++ {
			var match bool = false
			if from_b[0] == m[i] {
				match = true
				for j := 0; j < len(from_b); j++ {
					if from_b[j] != m[i+int64(j)] {
						match = false
						break
					}
				}

				if match {
					copy(m[i:], to_b)
					fmt.Fprintf(os.Stderr, "Patch @ 0x%08X [%s] -> [%s]\n", i, from, to)
					patched = true
				}
			}
		}
	} else {
		log.Fatalln(err)
	}

	return patched
}
