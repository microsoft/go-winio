package main

import (
	"fmt"
	"os"

	"github.com/Microsoft/go-winio/wim"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <WIM>\n", os.Args[0])
		os.Exit(1)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(file string) error {
	f, err := os.OpenFile(file, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	r, err := wim.NewReader(f)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "%#v\n%#v\n", r.Image[0], r.Image[0].Windows)
	dir, err := r.Image[0].Open()
	if err != nil {
		return err
	}
	return recur(dir)
}

func recur(dir *wim.File) error {
	files, err := dir.Readdir()
	if err != nil {
		return fmt.Errorf("cannot read %q: %w", dir.Name, err)
	}
	for _, f := range files {
		if f.IsDir() {
			if err := recur(f); err != nil {
				return fmt.Errorf("cannot recurse directory %q: %w", f.Name, err)
			}
		}
	}
	return nil
}
