package main

import (
	"github.com/tiqwab/x86-emulator"
	"os"
)

func main() {
	exitCode, _, err := x86_emulator.RunExe(os.Stdin)
	if err != nil {
		panic(err)
	}
	os.Exit(int(exitCode))
}
