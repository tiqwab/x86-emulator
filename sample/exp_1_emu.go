package main

import (
	"github.com/tiqwab/x86-emulator"
	"os"
	"fmt"
)

func main() {
	state, err := x86_emulator.RunExe(os.Stdin)
	if err != nil {
		panic(err)
	}
	fmt.Println(state)
}
