// Command icon-gen generates platform icon assets from the shared Status Deck mark.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"status-deck/desktop/internal/appicon"
)

func main() {
	var iconsetDir string
	var icoPath string
	flag.StringVar(&iconsetDir, "iconset", "", "output macOS .iconset directory")
	flag.StringVar(&icoPath, "ico", "", "output Windows .ico file")
	flag.Parse()

	if iconsetDir == "" && icoPath == "" {
		fatalf("set --iconset and/or --ico")
	}
	if iconsetDir != "" {
		if err := writeIconset(iconsetDir); err != nil {
			fatalf("generate iconset: %v", err)
		}
	}
	if icoPath != "" {
		if err := writeICO(icoPath); err != nil {
			fatalf("generate ico: %v", err)
		}
	}
}

func writeIconset(directory string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	for _, item := range []struct {
		name string
		size int
	}{
		{"icon_16x16.png", 16}, {"icon_16x16@2x.png", 32},
		{"icon_32x32.png", 32}, {"icon_32x32@2x.png", 64},
		{"icon_128x128.png", 128}, {"icon_128x128@2x.png", 256},
		{"icon_256x256.png", 256}, {"icon_256x256@2x.png", 512},
		{"icon_512x512.png", 512}, {"icon_512x512@2x.png", 1024},
	} {
		content, err := appicon.PNG(item.size)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, item.name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeICO(path string) error {
	content, err := appicon.ICO()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
