package magiskboot_test

import (
	"magiskboot"
	"testing"
)

func TestCehckFmt(t *testing.T) {
	t.Log("Test check fmt")

	tdata := []byte("\x1f\x8b\x00\x00\xff\xff\xff\xff")

	if ret := magiskboot.CheckFmt(tdata); ret != magiskboot.GZIP {
		t.Fatalf("CheckFmt failed, Except: GZIP:%v But:%v", magiskboot.GZIP, ret)
	}

	if ret := magiskboot.Fmt2Name(magiskboot.LZ4); ret != "lz4" {
		t.Fatalf("Fmt2Name failed, Except: lz4, But: %v", ret)
	}

	if ret := magiskboot.Name2Fmt("lz4"); ret != magiskboot.LZ4 {
		t.Fatalf("Name2Fmt failed, Except: %v, But: %v", magiskboot.LZ4, ret)
	}
}
