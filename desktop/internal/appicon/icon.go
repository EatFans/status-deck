// Package appicon renders the canonical Status Deck mark for tray and app icons.
package appicon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

const designSize = 18

type iconImage struct {
	size int
	png  []byte
}

// PNG renders the Status Deck icon at an exact square size.
func PNG(size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("icon size must be positive")
	}

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := func(value int) int {
		return int(math.Round(float64(value) * float64(size) / designSize))
	}

	background := color.RGBA{R: 24, G: 28, B: 36, A: 255}
	screen := color.RGBA{R: 54, G: 211, B: 153, A: 255}
	accent := color.RGBA{R: 96, G: 165, B: 250, A: 255}

	fillRoundedRect(img, scale(1), scale(2), scale(16), scale(13), scale(3), background)
	fillRect(img, scale(4), scale(5), scale(10), scale(2), screen)
	fillRect(img, scale(4), scale(9), scale(6), scale(2), accent)
	fillRect(img, scale(6), scale(15), scale(6), scale(1), background)

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, fmt.Errorf("encode icon PNG: %w", err)
	}
	return buffer.Bytes(), nil
}

// ICO returns a multi-resolution Windows icon containing the Status Deck mark.
func ICO() ([]byte, error) {
	images := make([]iconImage, 0, 4)
	for _, size := range []int{16, 32, 48, 256} {
		content, err := PNG(size)
		if err != nil {
			return nil, err
		}
		images = append(images, iconImage{size: size, png: content})
	}

	var output bytes.Buffer
	for _, value := range []uint16{0, 1, uint16(len(images))} {
		if err := binary.Write(&output, binary.LittleEndian, value); err != nil {
			return nil, fmt.Errorf("write ICO header: %w", err)
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
			return nil, fmt.Errorf("write ICO directory: %w", err)
		}
		offset += uint32(len(image.png))
	}
	for _, image := range images {
		if _, err := output.Write(image.png); err != nil {
			return nil, fmt.Errorf("write ICO image: %w", err)
		}
	}
	return output.Bytes(), nil
}

func fillRect(img *image.RGBA, x, y, width, height int, color color.RGBA) {
	for py := y; py < y+height; py++ {
		for px := x; px < x+width; px++ {
			img.SetRGBA(px, py, color)
		}
	}
}

func fillRoundedRect(img *image.RGBA, x, y, width, height, radius int, color color.RGBA) {
	for py := y; py < y+height; py++ {
		for px := x; px < x+width; px++ {
			left := px - x
			right := x + width - 1 - px
			top := py - y
			bottom := y + height - 1 - py

			if left < radius && top < radius && (left-radius)*(left-radius)+(top-radius)*(top-radius) > radius*radius {
				continue
			}
			if right < radius && top < radius && (right-radius)*(right-radius)+(top-radius)*(top-radius) > radius*radius {
				continue
			}
			if left < radius && bottom < radius && (left-radius)*(left-radius)+(bottom-radius)*(bottom-radius) > radius*radius {
				continue
			}
			if right < radius && bottom < radius && (right-radius)*(right-radius)+(bottom-radius)*(bottom-radius) > radius*radius {
				continue
			}

			img.SetRGBA(px, py, color)
		}
	}
}
