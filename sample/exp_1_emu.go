package main

import (
	"github.com/tiqwab/x86-emulator"
	"os"
	"fmt"
)

func main() {
	header := x86_emulator.ParseHeader(os.Stdin)
	fmt.Println(header)
}
