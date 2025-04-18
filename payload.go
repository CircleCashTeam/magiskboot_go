package magiskboot

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"

	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	update_engine "magiskboot/chromeos_update_engine"
)

func badPayload(msg string) error {
	return errors.New("invalid payload: " + msg)
}

const PAYLOAD_MAGIC string = "CrAU"

func doExtractBootFromPayload(
	in_path string,
	partition_name string,
	out_path string,
) error {
	var reader io.ReadSeekCloser = func() io.ReadSeekCloser {
		if in_path == "-" {
			return os.Stdin
		} else {
			fd, err := os.Open(in_path)
			if err != nil {
				log.Fatalln(err)
			}
			return fd
		}
	}()
	defer reader.Close()

	buf := make([]byte, 4)
	_, err := reader.Read(buf)
	if err != nil {
		log.Fatalln(err)
	}

	if !bytes.Equal(buf, []byte(PAYLOAD_MAGIC)) {
		return badPayload("invalid magic")
	}

	var version uint64
	binary.Read(reader, binary.BigEndian, &version)
	if version != 2 {
		return badPayload("unsupported version: " + strconv.FormatUint(version, 10))
	}

	var manifest_len uint64
	binary.Read(reader, binary.BigEndian, &manifest_len)
	if manifest_len == 0 {
		return badPayload("mainfest length is zero")
	}

	var manifest_sig_len uint32
	binary.Read(reader, binary.BigEndian, &manifest_sig_len)
	if manifest_sig_len == 0 {
		return badPayload("manifest signature length is zero")
	}

	buf = make([]byte, manifest_len)
	_, err = reader.Read(buf)
	if err != nil {
		log.Fatalln(err)
	}
	manifest := new(update_engine.DeltaArchiveManifest)
	if err := manifest.Unmarshal(buf); err != nil {
		log.Fatalln(err)
	}
	if manifest.GetMinorVersion() != 0 {
		return badPayload("delta payloads are not supported, please use a full payload file")
	}

	block_size := manifest.GetBlockSize()

	partition := func() *update_engine.PartitionUpdate {
		if partition_name == "" {
			var boot *update_engine.PartitionUpdate = nil
			for _, p := range manifest.Partitions {
				if p.GetPartitionName() == "init_boot" {
					boot = p
				}
			}
			if boot == nil {
				for _, p := range manifest.Partitions {
					if p.GetPartitionName() == "boot" {
						boot = p
					}
				}
			}
			if boot == nil {
				log.Fatalln(badPayload("boot partition not found"))
			}
			return boot
		} else {
			for _, p := range manifest.Partitions {
				if p.GetPartitionName() == partition_name {
					return p
				}
			}
			log.Fatalln(badPayload("partition " + partition_name + " not found"))

		}
		return nil
	}()

	var out_str string
	out_path = func() string {
		if out_path == "" {
			out_str = fmt.Sprintf("%s.img", partition.GetPartitionName())
			return out_str
		} else {
			return out_path
		}
	}()

	out_file, err := os.Create(out_path)
	if err != nil {
		log.Fatalln(err)
	}
	defer out_file.Close()

	// Skip the manifest signature
	reader.Seek(int64(manifest_sig_len), io.SeekCurrent)

	operations := slices.Clone(partition.Operations)
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].GetDataOffset() < operations[j].GetDataOffset()
	})

	curr_data_offset := int64(0)

	for _, operation := range operations {
		data_len := operation.GetDataLength()
		if data_len == 0 {
			return badPayload("data length not found")
		}
		data_offset := operation.GetDataOffset()
		if data_offset == 0 {
			return badPayload("data offset not found")
		}
		data_type := operation.GetType()

		buf := make([]byte, data_len)

		skip := data_offset - uint64(curr_data_offset)
		reader.Seek(int64(skip), io.SeekCurrent)
		reader.Read(buf)
		curr_data_offset = int64(data_offset) + int64(data_len)

		out_offset := operation.GetDstExtents()[0].GetStartBlock() * uint64(block_size)
		switch data_type {
		case update_engine.REPLACE:
			out_file.Seek(int64(out_offset), io.SeekStart)
			_, err := out_file.Write(buf)
			if err != nil {
				log.Fatalln(err)
			}
		case update_engine.ZERO:
			for _, ext := range operation.GetDstExtents() {
				out_seek := ext.GetStartBlock() * uint64(block_size)
				num_blocks := ext.GetNumBlocks()
				out_file.Seek(int64(out_seek), io.SeekStart)
				out_file.Write(make([]byte, num_blocks))
			}
		case update_engine.REPLACE_BZ,
			update_engine.REPLACE_XZ:
			out_file.Seek(int64(out_offset), io.SeekStart)
			if !DecompressToFd(buf, out_file) {
				return badPayload("decompression failed")
			}
		default:
			fmt.Fprintln(os.Stderr, "DATA_TYPE: ", data_type)
			return badPayload("unsupported operation type")
		}

	}
	return nil
}

func ExtractBootFromPayload(
	in_path,
	partition,
	out_path string,
) bool {
	in_path = strings.TrimRight(in_path, " ")
	partition = strings.TrimRight(partition, " ")
	out_path = strings.TrimRight(out_path, " ")

	if err := doExtractBootFromPayload(in_path, partition, out_path); err != nil {
		log.Println(err)
		return false
	}
	return true
}
