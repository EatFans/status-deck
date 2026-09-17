// Command icon-gen generates platform icon assets from the shared Status Deck mark.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"status-deck/desktop/internal/appicon"
)

type iconImage struct {
	size int
	png  []byte
}

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
	images := make([]iconImage, 0, 4)
	for _, size := range []int{16, 32, 48, 256} {
		content, err := appicon.PNG(size)
		if err != nil {
			return err
		}
		images = append(images, iconImage{size: size, png: content})
	}

	var output bytes.Buffer
	for _, value := range []uint16{0, 1, uint16(len(images))} {
		if err := binary.Write(&output, binary.LittleEndian, value); err != nil {
			return err
		}
	}

	offset := uint32(6 + 16*len(images))
	for _, image := range images {
		dimension := uint8(image.size)
		if image.size >= 256 {
			dimension = 0
		}
		entry := struct {
			Width       uint8
			Height      uint8
			ColorCount  uint8
			Reserved    uint8
			Planes      uint16
			BitCount    uint16
			BytesInRes  uint32
			ImageOffset uint32
	}{dimension, dimension, 0, 0, 1, 32, uint32(len(image.png)), offset}
		if err := binary.Write(&output, binary.LittleEndian, entry); err != nil {
			return err
		}
		offset += uint32(len(image.png))
	}
	for _, image := range images {
		if _, err := output.Write(image.png); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, output.Bytes(), 0o644)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
