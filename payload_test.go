package magiskboot_test

import (
	"magiskboot"
	"testing"
)

func TestPayload(t *testing.T) {
	t.Log("Test payload extract")

	magiskboot.ExtractBootFromPayload("payload.bin", "system", "")
}
