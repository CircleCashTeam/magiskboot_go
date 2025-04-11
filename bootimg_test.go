package magiskboot_test

import (
	"encoding/binary"
	"magiskboot"
	"reflect"
	"testing"
)

func TestAlign(t *testing.T) {
	t.Log("Test structure align size")

	tests := map[interface{}]int{
		magiskboot.MtkHdr{}:               512,
		magiskboot.DhtbHdr{}:              512,
		magiskboot.BlobHdr{}:              104,
		magiskboot.ZimageHdr{}:            52,
		magiskboot.AvbFooter{}:            64,
		magiskboot.AvbVBMetaImageHeader{}: 256,
		magiskboot.BootImgHdrV0{}:         1632,
		magiskboot.BootImgHdrV1{}:         1648,
		magiskboot.BootImgHdrV2{}:         1660,
		magiskboot.BootImgHdrPxa{}:        1640,
		magiskboot.BootImgHdrV3{}:         1580,
		magiskboot.BootImgHdrV4{}:         1584,
		magiskboot.BootImgHdrVndV3{}:      2112,
		magiskboot.BootImgHdrVndV4{}:      2128,
	}

	for v, s := range tests {
		rt := reflect.TypeOf(v)
		t.Logf("Check align of: %v", rt.Name())
		if ret := binary.Size(v); ret != s {
			t.Fatalf("Align mismatch at: %v, Except: %v, But: %v", rt.Name(), s, ret)
		}
	}
}
