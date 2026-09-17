// Package appicon renders the canonical Status Deck mark for tray and app icons.
package appicon

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

const designSize = 18

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
