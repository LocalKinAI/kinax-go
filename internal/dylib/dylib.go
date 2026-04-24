// Package dylib embeds the ObjC companion library so downstream users can
// simply `go get` kinax-go without building C code on their machine.
package dylib

import _ "embed"

// Bytes holds the embedded universal-Mach-O bytes of libkinax_sync.dylib.
//
//go:embed libkinax_sync.dylib
var Bytes []byte
