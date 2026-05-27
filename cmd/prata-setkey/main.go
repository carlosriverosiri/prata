// Command prata-setkey encrypts the supplied Berget API key with
// Windows DPAPI and saves it to %LOCALAPPDATA%\Prata\apikey.dat for
// later use by ptt-test (and the production daemon).
//
// Usage:
//
//	prata-setkey sk_ber_...            (key as argument)
//	prata-setkey                       (interactive prompt)
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/carlosriveros/prata/internal/auth"
)

func main() {
	var key string
	switch {
	case len(os.Args) >= 2:
		key = os.Args[1]
	default:
		fmt.Fprint(os.Stderr, "Enter Berget API key: ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "read: %v\n", err)
			os.Exit(1)
		}
		key = line
	}
	key = strings.TrimSpace(key)
	if key == "" {
		fmt.Fprintln(os.Stderr, "no key provided")
		os.Exit(1)
	}

	if err := auth.SaveAPIKey(key); err != nil {
		fmt.Fprintf(os.Stderr, "save: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "API key encrypted and saved to:", auth.KeyPath())
}
