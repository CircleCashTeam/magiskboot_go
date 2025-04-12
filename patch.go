package magiskboot

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/edsrzf/mmap-go"
)

// 定义要移除的模式
var (
	verityPatterns = [][]byte{
		[]byte("verifyatboot"),
		[]byte("verify"),
		[]byte("avb_keys"),
		[]byte("avb"),
		[]byte("support_scfs"),
		[]byte("fsverity"),
	}

	encryptionPatterns = [][]byte{
		[]byte("forceencrypt"),
		[]byte("forcefdeorfbe"),
		[]byte("fileencryption"),
	}
)

// PatchVerity 移除 verity 相关标记
func PatchVerity(fstabContent []byte) []byte {
	return patchFstab(fstabContent, verityPatterns)
}

// PatchEncryption 移除 encryption 相关标记
func PatchEncryption(fstabContent []byte) []byte {
	return patchFstab(fstabContent, encryptionPatterns)
}

// patchFstab 核心处理函数
func patchFstab(fstabContent []byte, patterns [][]byte) []byte {
	lines := bytes.Split(fstabContent, []byte{'\n'})
	var result [][]byte

	for _, line := range lines {
		if len(line) == 0 || line[0] == '#' {
			result = append(result, line)
			continue
		}

		// 分割每一行的字段
		fields := bytes.Fields(line)
		if len(fields) < 4 {
			result = append(result, line)
			continue
		}

		// 处理 fs_mgr_flags (第4个字段)
		flags := bytes.Split(fields[4], []byte{','})
		var newFlags [][]byte

		for _, flag := range flags {
			shouldRemove := false
			for _, pattern := range patterns {
				if bytes.HasPrefix(flag, pattern) {
					fmt.Printf("Remove pattern [%s]\n", flag)
					shouldRemove = true
					break
				}
			}
			if !shouldRemove {
				newFlags = append(newFlags, flag)
			}
		}

		// 重建行
		newLine := bytes.Join([][]byte{
			bytes.Join(fields[:4], []byte{' '}),
			bytes.Join(newFlags, []byte{','}),
		}, []byte{' '})

		// 如果有第5个字段，追加到行尾
		if len(fields) > 5 {
			newLine = append(newLine, ' ')
			newLine = append(newLine, bytes.Join(fields[5:], []byte{' '})...)
		}

		result = append(result, newLine)
	}

	return bytes.Join(result, []byte{'\n'})
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
