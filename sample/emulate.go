package main

import (
	"fmt"
	"github.com/tiqwab/x86-emulator"
	"log"
	"os"
)

func main() {
	exitCode, state, err := x86_emulator.RunExe(os.Stdin)
	if err != nil {
		log.Panicf("%+v", err)
	}
	fmt.Println()
	fmt.Printf("state: %+v\n", state)
	os.Exit(int(exitCode))
}
