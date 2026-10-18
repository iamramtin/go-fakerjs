// Command dylibs lists the dynamic libraries a Go binary links against (ELF, Mach-O or PE), which is the
// check `ldd` / `otool -L` performs, but usable for any target from any host.
package main

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"os"
)

func main() {
	for _, path := range os.Args[1:] {
		libs, kind, err := list(path)
		if err != nil {
			fmt.Printf("%s: %v\n", path, err)
			os.Exit(1)
		}

		fmt.Printf("%s: %s, %d dynamic libraries %v\n", path, kind, len(libs), libs)
	}
}

func list(path string) ([]string, string, error) {
	if f, err := elf.Open(path); err == nil {
		defer func() { _ = f.Close() }()

		l, err := f.ImportedLibraries()

		return l, "ELF", err
	}

	if f, err := macho.Open(path); err == nil {
		defer func() { _ = f.Close() }()

		l, err := f.ImportedLibraries()

		return l, "Mach-O", err
	}

	if f, err := pe.Open(path); err == nil {
		defer func() { _ = f.Close() }()

		l, err := f.ImportedLibraries()

		return l, "PE", err
	}

	return nil, "", errors.New("not an ELF, Mach-O or PE binary")
}
