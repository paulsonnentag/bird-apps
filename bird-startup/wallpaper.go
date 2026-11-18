package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// makeWallpaper crops the photo to the screen's aspect ratio (so we know
// exactly what will be visible) and draws the bird's name in the corner.
func makeWallpaper(photo []byte, screenW, screenH int, b *Bird) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(photo))
	if err != nil {
		return nil, err
	}
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	target := float64(screenW) / float64(screenH)
	crop := sb
	if float64(w)/float64(h) > target { // too wide: trim sides
		cw := int(float64(h) * target)
		x0 := sb.Min.X + (w-cw)/2
		crop = image.Rect(x0, sb.Min.Y, x0+cw, sb.Max.Y)
	} else { // too tall: trim top/bottom, keeping a bit more of the top
		ch := int(float64(w) / target)
		y0 := sb.Min.Y + (h-ch)*2/5
		crop = image.Rect(sb.Min.X, y0, sb.Max.X, y0+ch)
	}
	img := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(img, img.Bounds(), src, crop.Min, draw.Src)

	drawCaption(img, b)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func drawCaption(img *image.RGBA, b *Bird) {
	H := img.Bounds().Dy()
	W := img.Bounds().Dx()
	title := face(gobold.TTF, float64(H)/26)
	sub := face(goregular.TTF, float64(H)/58)
	if title == nil || sub == nil {
		return
	}
	defer title.Close()
	defer sub.Close()

	line2 := b.Scientific
	line3 := "Photo " + b.Attribution + " · iNaturalist"
	if b.Recordist != "" {
		line3 += "   Song: " + b.Recordist + " · xeno-canto"
	}

	right := W - W*3/100
	bottom := H - H*8/100 // stay clear of the taskbar
	y3 := bottom
	y2 := y3 - int(float64(H)/58*1.6)
	y1 := y2 - int(float64(H)/58*1.5)

	text(img, title, b.Common, right, y1)
	text(img, sub, line2, right, y2)
	text(img, sub, line3, right, y3)
}

func face(ttf []byte, size float64) font.Face {
	f, err := opentype.Parse(ttf)
	if err != nil {
		return nil
	}
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil
	}
	return fc
}

// text draws right-aligned white text with a soft dark shadow.
func text(img *image.RGBA, f font.Face, s string, right, baseline int) {
	d := &font.Drawer{Dst: img, Face: f}
	x := right - d.MeasureString(s).Round()
	off := max(1, f.Metrics().Height.Round()/18)
	d.Src = image.NewUniform(color.NRGBA{0, 0, 0, 90})
	for _, o := range [][2]int{{off, off}, {off * 2, off * 2}, {0, off}, {off, 0}} {
		d.Dot = fixed.P(x+o[0], baseline+o[1])
		d.DrawString(s)
	}
	d.Src = image.NewUniform(color.White)
	d.Dot = fixed.P(x, baseline)
	d.DrawString(s)
}
