package magiskboot_test

import (
	"bytes"
	"io"
	"magiskboot"
	"os"
	"testing"
)

func TestHexPatch(t *testing.T) {
	t.Log("Test HexPatch function")
	os.Remove("test.bin")
	if fd, err := os.Create("test.bin"); err != nil {
		t.Fatal(err)

	} else {
		fd.WriteString("12345678901234567890")
		fd.Close()
		magiskboot.HexPatch("test.bin", "31323334", "35363738")
		expect := []byte("56785678905678567890")
		if fd, err = os.Open("test.bin"); err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(fd)
		if !bytes.Equal(data, expect) {
			t.Fatalf("Except: %v\nBut: %v", expect, data)
		}
		defer fd.Close()
		defer os.Remove("test.bin")
	}
}

func TestRemovePatterns(t *testing.T) {
	t.Log("Test remove patterns function")

	tdata := []byte(`
# 123456
aa      aaaa          aaaaa
bb      bbbb          bbbbb  misc,forceencrypt=footer,whatever,blabla
`)

	except := []byte(`
# 123456
aa      aaaa          aaaaa
bb      bbbb          bbbbb  misc,whatever,blabla
`)

	newdata := magiskboot.PatchEncryption(tdata)
	if len(newdata) == len(tdata) {
		t.Fatal("Failed, size still equal")
	}

	if !bytes.Equal(newdata, except) {
		t.Fatalf("\nOrigin: %s\nExcept: %s\nBut: %s", tdata, except, newdata)
	}

}
