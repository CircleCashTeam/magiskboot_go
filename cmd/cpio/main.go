package main

import (
	"magiskboot/cpio"
	"os"
)

func main() {
	cpio.CpioCommands(os.Args[1:])
}
