//go:build tools

// This package contains imports to various tools used (eg, via `//go:generate`) within this repo.
//
// Calls to `go run <cmd/import/path>` (or `//go:generate go run <cmd/import/path>`) for go executables
// included here will use the version specified in `go.mod` and build the executable from vendored code.
//
// Based on golang [guidance].
//
// [guidance]: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
package tools

import _ "golang.org/x/tools/cmd/stringer"
