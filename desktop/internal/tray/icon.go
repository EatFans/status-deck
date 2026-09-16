package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func statusDeckIconPNG() []byte {
	const size = 18

	img := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := color.RGBA{R: 24, G: 28, B: 36, A: 255}
	screen := color.RGBA{R: 54, G: 211, B: 153, A: 255}
	accent := color.RGBA{R: 96, G: 165, B: 250, A: 255}

	fillRoundedRect(img, 1, 2, 16, 13, 3, bg)
	fillRect(img, 4, 5, 10, 2, screen)
	fillRect(img, 4, 9, 6, 2, accent)
	fillRect(img, 6, 15, 6, 1, bg)

	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return buffer.Bytes()
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			img.SetRGBA(px, py, c)
		}
	}
}

func fillRoundedRect(img *image.RGBA, x, y, w, h, r int, c color.RGBA) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			left := px - x
			right := x + w - 1 - px
			top := py - y
			bottom := y + h - 1 - py

			if left < r && top < r && (left-r)*(left-r)+(top-r)*(top-r) > r*r {
				continue
			}
			if right < r && top < r && (right-r)*(right-r)+(top-r)*(top-r) > r*r {
				continue
			}
			if left < r && bottom < r && (left-r)*(left-r)+(bottom-r)*(bottom-r) > r*r {
				continue
			}
			if right < r && bottom < r && (right-r)*(right-r)+(bottom-r)*(bottom-r) > r*r {
				continue
			}

			img.SetRGBA(px, py, c)
		}
	}
}
