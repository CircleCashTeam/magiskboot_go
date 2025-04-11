package magiskboot_test

import (
	"magiskboot"
	"os"
	"testing"
)

func TestCheckEnv(t *testing.T) {
	t.Log("Test check env function")

	os.Setenv("FOO", "true")
	os.Setenv("BAR", "false")

	t.Log("Test FOO:true")
	if magiskboot.CheckEnv("FOO") != true {
		t.Fatalf("CheckEnv failed")
	}

	t.Log("Test BAR:false")
	if magiskboot.CheckEnv("BAR") != false {
		t.Fatalf("CheckEnv failed")
	}
}
