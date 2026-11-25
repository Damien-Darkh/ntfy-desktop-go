package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"fyne.io/fyne/v2"

	"github.com/Damien-Darkh/ntfy-desktop-go/assets"
)

// trayIcons returns the plain tray icon and a copy with a red "unread" dot.
// Both are built once at startup; the tray just switches between them.
func trayIcons() (normal, unread fyne.Resource, err error) {
	base, err := png.Decode(bytes.NewReader(assets.TrayIcon))
	if err != nil {
		return nil, nil, err
	}

	// Copy the base icon into an editable image and draw the dot on top.
	dst := image.NewNRGBA(base.Bounds())
	draw.Draw(dst, dst.Bounds(), base, base.Bounds().Min, draw.Src)
	drawDot(dst)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, nil, err
	}
	return fyne.NewStaticResource("tray.png", assets.TrayIcon),
		fyne.NewStaticResource("tray-unread.png", buf.Bytes()),
		nil
}

// drawDot paints a red circle with a white ring in the top-right corner.
// A tray icon is far too small for a readable number, so the count is shown
// in the tray menu instead.
func drawDot(img *image.NRGBA) {
	b := img.Bounds()
	size := float64(b.Dx())
	cx, cy := size*0.74, size*0.26
	radius := size * 0.20
	ring := radius + size*0.03

	red := color.NRGBA{R: 0xE5, G: 0x39, B: 0x35, A: 0xFF}
	white := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d2 := dx*dx + dy*dy
			switch {
			case d2 <= radius*radius:
				img.SetNRGBA(x, y, red)
			case d2 <= ring*ring:
				img.SetNRGBA(x, y, white)
			}
		}
	}
}
