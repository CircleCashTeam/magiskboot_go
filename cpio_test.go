package magiskboot_test

import (
	cpio "magiskboot/cpio"
	"os"
	"testing"
)

func TestCpio(t *testing.T) {
	cpio := cpio.NewCpio()

	err := cpio.LoadFromFile("test.cpio")
	if err != nil {
		t.Fatalf("Failed with %v", err)
	}
	defer cpio.Close()
	t.Logf("entries: %d", len(cpio.Entries))
	for _, v := range cpio.Keys {
		t.Logf("entry: %v: %v", v, cpio.Entries[v])
	}

	os.Remove("dump.cpio")
	cpio.Rm("test", true)

	err = cpio.Add(0755, "test/README.md", "README.md")
	if err != nil {
		t.Fatal("Failed to add file", err)
	}

	err = cpio.Dump("dump.cpio")
	if err != nil {
		t.Fatalf("Failed with %v", err)
	}
}

func TestRamdisk(t *testing.T) {
	cpio.CpioCommands([]string{
		"ramdisk.cpio",
		"ls",
	})

}
