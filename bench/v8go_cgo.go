//go:build v8go

package bench

// v8go's prebuilt V8 for macOS needs CoreFoundation at link time. Declaring it here lets `make bench-v8go` link
// without CGO_LDFLAGS in the environment. On other platforms the directive does nothing.

// #cgo darwin LDFLAGS: -framework CoreFoundation
import "C"
