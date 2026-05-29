// Package icon embeds the Prata application icon so binaries can use it
// without resolving a file path at runtime.
package icon

import _ "embed"

// ICO holds the raw bytes of the application icon in .ico format.
//
//go:embed Prata.ico
var ICO []byte
