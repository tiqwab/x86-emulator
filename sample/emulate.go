package main

import (
	"github.com/tiqwab/x86-emulator"
	"log"
	"os"
)

func main() {
	exitCode, _, err := x86_emulator.RunExe(os.Stdin)
	if err != nil {
		log.Panicf("%+v", err)
	}
	os.Exit(int(exitCode))
}
