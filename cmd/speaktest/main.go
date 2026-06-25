// Command speaktest verifies internal/speak: it says one sentence out loud via
// SAPI and exits. Usage: speaktest ["custom text"].
package main

import (
	"fmt"
	"os"

	"github.com/carlosriveros/prata/internal/speak"
)

func main() {
	text := "Inget ljud. Är mikrofonen påslagen?"
	if len(os.Args) >= 2 {
		text = os.Args[1]
	}
	fmt.Printf("speaking: %q\n", text)
	speak.Say(text)
	fmt.Println("done")
}
