package magiskboot

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const (
	HEADER_FILE     = "header"
	KERNEL_FILE     = "kernel"
	RAMDISK_FILE    = "ramdisk.cpio"
	VND_RAMDISK_DIR = "vendor_ramdisk"
	SECOND_FILE     = "second"
	EXTRA_FILE      = "extra"
	KER_DTB_FILE    = "kernel_dtb"
	RECV_DTBO_FILE  = "recovery_dtbo"
	DTB_FILE        = "dtb"
	BOOTCONFIG_FILE = "bootconfig"
	NEW_BOOT        = "new-boot.img"
)

func CheckEnv(key string) bool {
	value, ret := os.LookupEnv(key)
	if ret {
		if value == "true" {
			return true
		}
	}
	return false
}

func printFormats() {
	for f := GZIP; f < LZOP; f++ {
		fmt.Fprintf(os.Stderr, "%s ", Fmt2Name(format_t(f)))
	}
}

func Usage() {
	fmt.Fprintf(os.Stderr, `MagiskBoot - Boot Image Modification Tool

Usage: %s <action> [args...]

Supported actions:
  unpack [-n] [-h] <bootimg>
    Unpack <bootimg> to its individual components, each component to
    a file with its corresponding file name in the current directory.
    Supported components: kernel, kernel_dtb, ramdisk.cpio, second,
    dtb, extra, and recovery_dtbo.
    By default, each component will be decompressed on-the-fly.
    If '-n' is provided, all decompression operations will be skipped;
    each component will remain untouched, dumped in its original format.
    If '-h' is provided, the boot image header information will be
    dumped to the file 'header', which can be used to modify header
    configurations during repacking.
    Return values:
    0:valid    1:error    2:chromeos

  repack [-n] <origbootimg> [outbootimg]
    Repack boot image components using files from the current directory
    to [outbootimg], or 'new-boot.img' if not specified. Current directory
    should only contain required files for [outbootimg], or incorrect
    [outbootimg] may be produced.
    <origbootimg> is the original boot image used to unpack the components.
    By default, each component will be automatically compressed using its
    corresponding format detected in <origbootimg>. If a component file
    in the current directory is already compressed, then no addition
    compression will be performed for that specific component.
    If '-n' is provided, all compression operations will be skipped.
    If env variable PATCHVBMETAFLAG is set to true, all disable flags in
    the boot image's vbmeta header will be set.

  verify <bootimg> [x509.pem]
    Check whether the boot image is signed with AVB 1.0 signature.
    Optionally provide a certificate to verify whether the image is
    signed by the public key certificate.
    Return value:
    0:valid    1:error

  sign <bootimg> [name] [x509.pem pk8]
    Sign <bootimg> with AVB 1.0 signature.
    Optionally provide the name of the image (default: '/boot').
    Optionally provide the certificate/private key pair for signing.
    If the certificate/private key pair is not provided, the AOSP
    verity key bundled in the executable will be used.

  extract <payload.bin> [partition] [outfile]
    Extract [partition] from <payload.bin> to [outfile].
    If [outfile] is not specified, then output to '[partition].img'.
    If [partition] is not specified, then attempt to extract either
    'init_boot' or 'boot'. Which partition was chosen can be determined
    by whichever 'init_boot.img' or 'boot.img' exists.
    <payload.bin> can be '-' to be STDIN.

  hexpatch <file> <hexpattern1> <hexpattern2>
    Search <hexpattern1> in <file>, and replace it with <hexpattern2>

  cpio <incpio> [commands...]
    Do cpio commands to <incpio> (modifications are done in-place).
    Each command is a single argument; add quotes for each command.
    See "cpio --help" for supported commands.

  dtb <file> <action> [args...]
    Do dtb related actions to <file>.
    See "dtb --help" for supported actions.

  split [-n] <file>
    Split image.*-dtb into kernel + kernel_dtb.
    If '-n' is provided, decompression operations will be skipped;
    the kernel will remain untouched, split in its original format.

  sha1 <file>
    Print the SHA1 checksum for <file>

  cleanup
    Cleanup the current working directory

  compress[=format] <infile> [outfile]
    Compress <infile> with [format] to [outfile].
    <infile>/[outfile] can be '-' to be STDIN/STDOUT.
    If [format] is not specified, then gzip will be used.
    If [outfile] is not specified, then <infile> will be replaced
    with another file suffixed with a matching file extension.
    Supported formats: `, os.Args[0])

	printFormats()

	fmt.Fprintf(os.Stderr, `
	
  decompress <infile> [outfile]
    Detect format and decompress <infile> to [outfile].
    <infile>/[outfile] can be '-' to be STDIN/STDOUT.
    If [outfile] is not specified, then <infile> will be replaced
    with another file removing its archive format file extension.
    Supported formats: `)

	printFormats()

	fmt.Fprintf(os.Stderr, "\n\n")
	os.Exit(1)
}

func Main(args []string) {
	if len(args) < 2 {
		Usage()
	}

	// Skip '--' for backwards compatibility
	action := strings.TrimLeft(args[1], "-")

	notImplError := errors.New("not impl yet")

	if action == "cleanup" {
		fmt.Fprintf(os.Stderr, "Cleaning up...\n")
		for _, f := range []string{
			HEADER_FILE,
			KERNEL_FILE,
			RAMDISK_FILE,
			SECOND_FILE,
			KER_DTB_FILE,
			EXTRA_FILE,
			RECV_DTBO_FILE,
			DTB_FILE,
			BOOTCONFIG_FILE,
		} {
			os.Remove(f)
		}
		os.RemoveAll(VND_RAMDISK_DIR)
	} else if len(args) > 2 && action == "sha1" {
		fd, err := os.Open(args[2])
		if err != nil {
			log.Fatalln("Error:", err)
		}
		defer fd.Close()

		hash := sha1.New()
		if _, err := io.Copy(hash, fd); err != nil {
			panic(err)
		}
		_sha1 := hash.Sum(nil)
		fmt.Printf("%x\n", _sha1)
	} else if len(args) > 2 && action == "split" { // TODO:
		if args[2] == "-n" {
			if len(args) == 3 {
				Usage()
			}
			os.Exit(SplitImageDtb(args[3], true))
		} else {
			os.Exit(SplitImageDtb(args[2], false))
		}
	} else if len(args) > 2 && action == "unpack" {
		panic(notImplError)
	} else if len(args) > 2 && action == "repack" {
		panic(notImplError)
	} else if len(args) > 2 && action == "verify" {
		panic(notImplError)
	} else if len(args) > 2 && action == "sign" {
		if len(args) == 5 {
			Usage()
		}
		panic(notImplError)
	} else if len(args) > 2 && action == "decompress" {
		Decompress(args[2], func() string {
			if len(args) > 3 {
				return args[3]
			}
			return ""
		}())
	} else if len(args) > 2 && strings.HasPrefix(action, "compress") {
		Compress(func() string {
			if len(action) > 8 && action[8] == '=' {
				return action[9:]
			}
			return "gzip"
		}(),
			args[2],
			func() string {
				if len(args) > 3 {
					return args[3]
				}
				return ""
			}(),
		)
	} else if len(args) > 4 && action == "hexpatch" {
		os.Exit(func() int {
			if HexPatch(args[2], args[3], args[4]) {
				return 0
			} else {
				return 1
			}
		}())
	} else if len(args) > 2 && action == "cpio" {
		CpioCommands(args[2:])
		os.Exit(0)
	} else if len(args) > 2 && action == "dtb" {
		panic(notImplError)
	} else if len(args) > 2 && action == "extract" {
		os.Exit(func() int {
			if ExtractBootFromPayload(
				args[2],
				func() string {
					if len(args) > 3 {
						return args[3]
					} else {
						return ""
					}
				}(),
				func() string {
					if len(args) > 4 {
						return args[4]
					} else {
						return ""
					}
				}(),
			) {
				return 0
			} else {
				return 1
			}
		}())
	} else {
		Usage()
	}
}
