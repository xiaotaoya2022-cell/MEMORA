//go:build ignore

// Tool mkassets generates Windows installer and tray icon assets from the
// official files in assets/brand/.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		die("getwd: %v", err)
	}

	iconSrc := loadImg(filepath.Join(root, "assets", "brand", "icon.png"))
	coverSrc := loadImg(filepath.Join(root, "assets", "brand", "cover.jpg"))
	masterIcon := prepareAppIconMaster(iconSrc)

	icoPath := filepath.Join(root, "assets", "brand", "memora.ico")
	if err := writeICO(icoPath, masterIcon); err != nil {
		die("write ico: %v", err)
	}
	fmt.Printf("wrote %s\n", icoPath)

	cmdIcoPath := filepath.Join(root, "cmd", "memora", "memora.ico")
	if icoRaw, err := os.ReadFile(icoPath); err != nil {
		die("read ico for cmd: %v", err)
	} else if err := os.WriteFile(cmdIcoPath, icoRaw, 0o644); err != nil {
		die("write cmd ico: %v", err)
	}
	fmt.Printf("wrote %s\n", cmdIcoPath)

	trayPath := filepath.Join(root, "cmd", "memora", "memora_tray.png")
	trayImg := resizeArea(masterIcon, 64, 64)
	sharpenNRGBA(trayImg, 0.35)
	if err := writePNG(trayPath, trayImg); err != nil {
		die("write tray png: %v", err)
	}
	fmt.Printf("wrote %s\n", trayPath)

	logoPath := filepath.Join(root, "cmd", "memora", "memora_logo.png")
	logoImg := resizeArea(masterIcon, 256, 256)
	if err := writePNG(logoPath, logoImg); err != nil {
		die("write logo png: %v", err)
	}
	fmt.Printf("wrote %s\n", logoPath)

	wizardPath := filepath.Join(root, "packaging", "windows", "wizard.bmp")
	wizardImg := buildWizardImage(coverSrc)
	if err := writeBMP24(wizardPath, wizardImg); err != nil {
		die("write wizard bmp: %v", err)
	}
	fmt.Printf("wrote %s\n", wizardPath)

	headerPath := filepath.Join(root, "packaging", "windows", "header.bmp")
	headerImg := buildHeaderImage(iconSrc)
	if err := writeBMP24(headerPath, headerImg); err != nil {
		die("write header bmp: %v", err)
	}
	fmt.Printf("wrote %s\n", headerPath)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func loadImg(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		die("open %s: %v", path, err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		die("decode %s: %v", path, err)
	}
	return img
}

// resizeArea performs box/area-averaged resampling with premultiplied alpha so
// downscaled brand ribbons remain smooth and crisp without jagged edges.
func resizeArea(src image.Image, dstW, dstH int) *image.NRGBA {
	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)

	for dy := 0; dy < dstH; dy++ {
		sy0 := float64(dy) * scaleY
		sy1 := float64(dy+1) * scaleY
		iy0 := int(math.Floor(sy0))
		iy1 := int(math.Ceil(sy1))
		if iy1 > srcH {
			iy1 = srcH
		}
		for dx := 0; dx < dstW; dx++ {
			sx0 := float64(dx) * scaleX
			sx1 := float64(dx+1) * scaleX
			ix0 := int(math.Floor(sx0))
			ix1 := int(math.Ceil(sx1))
			if ix1 > srcW {
				ix1 = srcW
			}

			var rSum, gSum, bSum, aSum, wSum float64
			for sy := iy0; sy < iy1; sy++ {
				wy := math.Min(float64(sy+1), sy1) - math.Max(float64(sy), sy0)
				if wy <= 0 {
					continue
				}
				for sx := ix0; sx < ix1; sx++ {
					wx := math.Min(float64(sx+1), sx1) - math.Max(float64(sx), sx0)
					if wx <= 0 {
						continue
					}
					w := wx * wy
					c := color.NRGBAModel.Convert(src.At(b.Min.X+sx, b.Min.Y+sy)).(color.NRGBA)
					af := float64(c.A) / 255.0
					rSum += float64(c.R) * af * w
					gSum += float64(c.G) * af * w
					bSum += float64(c.B) * af * w
					aSum += af * w
					wSum += w
				}
			}

			if wSum <= 0 || aSum <= 0 {
				dst.SetNRGBA(dx, dy, color.NRGBA{})
				continue
			}
			avgA := aSum / wSum
			r := clampU8(rSum / aSum)
			g := clampU8(gSum / aSum)
			bl := clampU8(bSum / aSum)
			dst.SetNRGBA(dx, dy, color.NRGBA{R: r, G: g, B: bl, A: clampU8(avgA * 255.0)})
		}
	}
	return dst
}

func clampU8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}

func buildWizardImage(cover image.Image) *image.NRGBA {
	const w, h = 164, 314
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))

	// Fill backdrop gradient matching the deep dark edges of cover.jpg (#0D1017 -> #090C12).
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		r := clampU8(13*(1-t) + 9*t)
		g := clampU8(16*(1-t) + 12*t)
		b := clampU8(23*(1-t) + 18*t)
		for x := 0; x < w; x++ {
			dst.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Crop center focal region (ribbon M + MEMORA wordmark + subtitle) from cover.jpg.
	cb := cover.Bounds()
	cropRect := image.Rect(
		cb.Min.X+int(float64(cb.Dx())*0.18),
		cb.Min.Y+int(float64(cb.Dy())*0.04),
		cb.Min.X+int(float64(cb.Dx())*0.82),
		cb.Min.Y+int(float64(cb.Dy())*0.92),
	)
	sub := subImage(cover, cropRect)
	fittedH := int(math.Round(float64(w) * float64(cropRect.Dy()) / float64(cropRect.Dx())))
	resized := resizeArea(sub, w, fittedH)

	// Center vertically and softly feather top/bottom edges into the backdrop.
	offsetY := (h - fittedH) / 2
	const fadeRows = 40.0
	for y := 0; y < fittedH; y++ {
		dy := offsetY + y
		if dy < 0 || dy >= h {
			continue
		}
		alpha := 1.0
		if float64(y) < fadeRows {
			alpha = float64(y) / fadeRows
		} else if float64(fittedH-1-y) < fadeRows {
			alpha = float64(fittedH-1-y) / fadeRows
		}
		for x := 0; x < w; x++ {
			bg := dst.NRGBAAt(x, dy)
			fg := resized.NRGBAAt(x, y)
			dst.SetNRGBA(x, dy, color.NRGBA{
				R: clampU8(float64(bg.R)*(1-alpha) + float64(fg.R)*alpha),
				G: clampU8(float64(bg.G)*(1-alpha) + float64(fg.G)*alpha),
				B: clampU8(float64(bg.B)*(1-alpha) + float64(fg.B)*alpha),
				A: 255,
			})
		}
	}
	return dst
}

func buildHeaderImage(icon image.Image) *image.NRGBA {
	const w, h = 150, 57
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	mark := resizeArea(icon, 45, 45)
	ox, oy := w-45-8, (h-45)/2
	for y := 0; y < 45; y++ {
		for x := 0; x < 45; x++ {
			fg := mark.NRGBAAt(x, y)
			if fg.A == 0 {
				continue
			}
			a := float64(fg.A) / 255.0
			dst.SetNRGBA(ox+x, oy+y, color.NRGBA{
				R: clampU8(255*(1-a) + float64(fg.R)*a),
				G: clampU8(255*(1-a) + float64(fg.G)*a),
				B: clampU8(255*(1-a) + float64(fg.B)*a),
				A: 255,
			})
		}
	}
	return dst
}

func subImage(src image.Image, r image.Rectangle) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.Set(x-r.Min.X, y-r.Min.Y, src.At(x, y))
		}
	}
	return dst
}

// prepareAppIconMaster builds a crisp 512x512 master icon on a clean #FFFFFF squircle
// with a subtle #CBD5E1 border ring, cropped/enlarged M ribbon (~86% width), and enhanced contrast.
func prepareAppIconMaster(src image.Image) *image.NRGBA {
	const size = 512
	const radius = 110.0

	b := src.Bounds()
	crop := image.Rect(
		b.Min.X+int(math.Round(float64(b.Dx())*0.09)),
		b.Min.Y+int(math.Round(float64(b.Dy())*0.09)),
		b.Min.X+int(math.Round(float64(b.Dx())*0.91)),
		b.Min.Y+int(math.Round(float64(b.Dy())*0.91)),
	)
	cropped := subImage(src, crop)

	ribbonSize := int(math.Round(float64(size) * 0.86))
	ribbon := resizeArea(cropped, ribbonSize, ribbonSize)
	enhanceRibbonContrast(ribbon)

	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	ox := (size - ribbonSize) / 2
	oy := (size - ribbonSize) / 2
	half := float64(size)/2.0 - 0.5
	innerHalf := half - radius

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := math.Abs(float64(x)-half) - innerHalf
			dy := math.Abs(float64(y)-half) - innerHalf
			ax := math.Max(dx, 0)
			ay := math.Max(dy, 0)
			dist := math.Hypot(ax, ay) + math.Min(math.Max(dx, dy), 0) - radius

			// Smooth outer edge AA (1px band).
			alpha := clampU8((0.5 - dist) * 255.0)
			if alpha < 8 {
				dst.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 0})
				continue
			}

			r, g, bl := 255.0, 255.0, 255.0
			rx, ry := x-ox, y-oy
			if rx >= 0 && rx < ribbonSize && ry >= 0 && ry < ribbonSize {
				fc := ribbon.NRGBAAt(rx, ry)
				if fc.A > 0 {
					fa := float64(fc.A) / 255.0
					r = r*(1-fa) + float64(fc.R)*fa
					g = g*(1-fa) + float64(fc.G)*fa
					bl = bl*(1-fa) + float64(fc.B)*fa
				}
			}

			// Subtle 1.5px #CBD5E1 (203, 213, 225) border ring so white squircle reads cleanly on light taskbars.
			if dist > -1.75 && dist <= 0.35 {
				ringW := 1.0 - math.Min(1.0, math.Max(0.0, (-0.15-dist)/1.6))
				if dist > -0.25 {
					ringW = 1.0
				}
				r = r*(1-ringW*0.85) + 203.0*(ringW*0.85)
				g = g*(1-ringW*0.85) + 213.0*(ringW*0.85)
				bl = bl*(1-ringW*0.85) + 225.0*(ringW*0.85)
			}

			dst.SetNRGBA(x, y, color.NRGBA{
				R: clampU8(r),
				G: clampU8(g),
				B: clampU8(bl),
				A: alpha,
			})
		}
	}
	return dst
}

// enhanceRibbonContrast deepens midtones/shadows toward #0F172A while keeping white background/highlights bright.
func enhanceRibbonContrast(img *image.NRGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A < 8 {
				continue
			}
			luma := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255.0
			if luma >= 0.96 {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: c.A})
				continue
			}
			// Pull dark/mid tones cleanly toward #0F172A (15, 23, 42) using a contrast S-curve.
			mapped := math.Pow(luma, 1.48)
			t := 1.0 - mapped
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			r := 255.0*(1-t) + 15.0*t
			g := 255.0*(1-t) + 23.0*t
			bl := 255.0*(1-t) + 42.0*t
			img.SetNRGBA(x, y, color.NRGBA{
				R: clampU8(r),
				G: clampU8(g),
				B: clampU8(bl),
				A: c.A,
			})
		}
	}
}

// sharpenNRGBA applies a gentle 3x3 unsharp mask so small icons (<=64px) stay razor crisp.
func sharpenNRGBA(img *image.NRGBA, strength float64) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	orig := image.NewNRGBA(b)
	copy(orig.Pix, img.Pix)

	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			c := orig.NRGBAAt(x, y)
			if c.A < 32 {
				continue
			}
			var rAvg, gAvg, bAvg float64
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nc := orig.NRGBAAt(x+dx, y+dy)
					rAvg += float64(nc.R)
					gAvg += float64(nc.G)
					bAvg += float64(nc.B)
				}
			}
			rAvg /= 9.0
			gAvg /= 9.0
			bAvg /= 9.0
			r := float64(c.R) + (float64(c.R)-rAvg)*strength
			g := float64(c.G) + (float64(c.G)-gAvg)*strength
			bl := float64(c.B) + (float64(c.B)-bAvg)*strength
			img.SetNRGBA(x, y, color.NRGBA{
				R: clampU8(r),
				G: clampU8(g),
				B: clampU8(bl),
				A: c.A,
			})
		}
	}
}

// writeICO encodes small sizes (16..48) as 32-bit DIB BMP frames for universal
// NSIS/Explorer compatibility, and large sizes (64..256) as crisp PNG frames.
func writeICO(path string, src image.Image) error {
	type frameSpec struct {
		size  int
		asPNG bool
	}
	specs := []frameSpec{
		{16, false},
		{24, false},
		{32, false},
		{48, false},
		{64, true},
		{128, true},
		{256, true},
	}

	frames := make([][]byte, len(specs))
	for i, sp := range specs {
		img := resizeArea(src, sp.size, sp.size)
		if sp.size <= 64 {
			sharpenNRGBA(img, 0.35)
		}
		if sp.asPNG {
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				return err
			}
			frames[i] = buf.Bytes()
		} else {
			frames[i] = encodeIconDIB(img)
		}
	}

	var out bytes.Buffer
	// ICONDIR: Reserved(2)=0, Type(2)=1 (ICO), Count(2)
	_ = binary.Write(&out, binary.LittleEndian, uint16(0))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(len(specs)))

	offset := uint32(6 + 16*len(specs))
	for i, sp := range specs {
		dim := uint8(sp.size)
		if sp.size >= 256 {
			dim = 0
		}
		out.WriteByte(dim) // bWidth
		out.WriteByte(dim) // bHeight
		out.WriteByte(0)   // bColorCount
		out.WriteByte(0)   // bReserved
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))                  // wPlanes
		_ = binary.Write(&out, binary.LittleEndian, uint16(32))                 // wBitCount
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(frames[i])))     // dwBytesInRes
		_ = binary.Write(&out, binary.LittleEndian, offset)                     // dwImageOffset
		offset += uint32(len(frames[i]))
	}
	for _, f := range frames {
		out.Write(f)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

func encodeIconDIB(img *image.NRGBA) []byte {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	andStride := ((w + 31) / 32) * 4
	xorSize := w * h * 4
	andSize := andStride * h

	var buf bytes.Buffer
	// BITMAPINFOHEADER (40 bytes)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(40))
	_ = binary.Write(&buf, binary.LittleEndian, int32(w))
	_ = binary.Write(&buf, binary.LittleEndian, int32(h*2)) // XOR + AND mask height
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0)) // BI_RGB
	_ = binary.Write(&buf, binary.LittleEndian, uint32(xorSize+andSize))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))

	// Bottom-up BGRA pixels. Clear RGB/A when nearly transparent to prevent dark edge halos.
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			if c.A < 24 {
				buf.Write([]byte{0, 0, 0, 0})
			} else {
				buf.Write([]byte{c.B, c.G, c.R, c.A})
			}
		}
	}
	// Bottom-up 1-bit AND mask.
	row := make([]byte, andStride)
	for y := h - 1; y >= 0; y-- {
		for i := range row {
			row[i] = 0
		}
		for x := 0; x < w; x++ {
			if img.NRGBAAt(x, y).A < 24 {
				row[x/8] |= 1 << uint(7-(x%8))
			}
		}
		buf.Write(row)
	}
	return buf.Bytes()
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeBMP24(path string, img *image.NRGBA) error {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	rowStride := ((w*3 + 3) / 4) * 4
	pixSize := rowStride * h
	fileSize := 14 + 40 + pixSize

	var buf bytes.Buffer
	// BITMAPFILEHEADER
	buf.Write([]byte{'B', 'M'})
	_ = binary.Write(&buf, binary.LittleEndian, uint32(fileSize))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(14+40))

	// BITMAPINFOHEADER
	_ = binary.Write(&buf, binary.LittleEndian, uint32(40))
	_ = binary.Write(&buf, binary.LittleEndian, int32(w))
	_ = binary.Write(&buf, binary.LittleEndian, int32(h))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(24))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(pixSize))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))

	pad := make([]byte, rowStride-w*3)
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			buf.Write([]byte{c.B, c.G, c.R})
		}
		if len(pad) > 0 {
			buf.Write(pad)
		}
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
