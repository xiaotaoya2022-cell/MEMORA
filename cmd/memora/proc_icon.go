package main

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"memora/internal/proc"
)

const (
	processQueryLimitedInformation = 0x1000
	shgfiIcon                      = 0x000000100
	shgfiLargeIcon                 = 0x000000000
	dibRGBColors                   = 0
	biRGB                          = 0
	maxProcIconCache               = 512
)

var (
	iconShell32 = syscall.NewLazyDLL("shell32.dll")
	iconUser32  = syscall.NewLazyDLL("user32.dll")
	iconGdi32   = syscall.NewLazyDLL("gdi32.dll")
	iconKernel  = syscall.NewLazyDLL("kernel32.dll")

	procSHGetFileInfoW             = iconShell32.NewProc("SHGetFileInfoW")
	procIconGetIconInfo            = iconUser32.NewProc("GetIconInfo")
	procIconDestroyIcon            = iconUser32.NewProc("DestroyIcon")
	procIconCreateCompatibleDC     = iconGdi32.NewProc("CreateCompatibleDC")
	procIconDeleteDC               = iconGdi32.NewProc("DeleteDC")
	procIconGetObjectW             = iconGdi32.NewProc("GetObjectW")
	procIconGetDIBits              = iconGdi32.NewProc("GetDIBits")
	procIconDeleteObject           = iconGdi32.NewProc("DeleteObject")
	procIconOpenProcess            = iconKernel.NewProc("OpenProcess")
	procIconQueryFullProcessImage  = iconKernel.NewProc("QueryFullProcessImageNameW")
	procIconCloseHandle            = iconKernel.NewProc("CloseHandle")

	procIconMu    sync.RWMutex
	procIconCache = map[string][]byte{}
)

type shFileInfoW struct {
	HIcon         uintptr
	IIcon         int32
	DwAttributes  uint32
	SzDisplayName [260]uint16
	SzTypeName    [80]uint16
}

type winIconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type winBitmap struct {
	BmType       int32
	BmWidth      int32
	BmHeight     int32
	BmWidthBytes int32
	BmPlanes     uint16
	BmBitsPixel  uint16
	BmBits       uintptr
}

type winBitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

func (d *dashboardServer) handleProcIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pidVal, _ := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("pid")), 10, 32)
	pid := uint32(pidVal)
	name := strings.TrimSpace(r.URL.Query().Get("name"))

	exePath := queryProcessExePath(pid)
	if exePath == "" && name != "" {
		if procs, _, err := proc.ListAll(); err == nil {
			for _, p := range procs {
				if strings.EqualFold(p.Name, name) {
					if pth := queryProcessExePath(p.PID); pth != "" {
						exePath = pth
						break
					}
				}
			}
		}
	}
	if exePath == "" {
		http.NotFound(w, r)
		return
	}

	key := strings.ToLower(exePath)
	procIconMu.RLock()
	cached, ok := procIconCache[key]
	procIconMu.RUnlock()
	if ok {
		if len(cached) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(cached)
		return
	}

	pngBytes := extractExeIconPNG(exePath)
	procIconMu.Lock()
	if len(procIconCache) >= maxProcIconCache {
		procIconCache = map[string][]byte{}
	}
	procIconCache[key] = pngBytes
	procIconMu.Unlock()

	if len(pngBytes) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(pngBytes)
}

func queryProcessExePath(pid uint32) string {
	if pid <= 4 {
		return ""
	}
	h, _, _ := procIconOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procIconCloseHandle.Call(h)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := procIconQueryFullProcessImage.Call(
		h,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 || size == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func extractExeIconPNG(exePath string) []byte {
	pathPtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return nil
	}
	var shfi shFileInfoW
	r, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&shfi)),
		unsafe.Sizeof(shfi),
		shgfiIcon|shgfiLargeIcon,
	)
	if r == 0 || shfi.HIcon == 0 {
		return nil
	}
	defer procIconDestroyIcon.Call(shfi.HIcon)

	var ii winIconInfo
	ok, _, _ := procIconGetIconInfo.Call(shfi.HIcon, uintptr(unsafe.Pointer(&ii)))
	if ok == 0 {
		return nil
	}
	if ii.HbmMask != 0 {
		defer procIconDeleteObject.Call(ii.HbmMask)
	}
	if ii.HbmColor != 0 {
		defer procIconDeleteObject.Call(ii.HbmColor)
	}
	if ii.HbmColor == 0 {
		return nil
	}

	var bm winBitmap
	if ret, _, _ := procIconGetObjectW.Call(ii.HbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); ret == 0 {
		return nil
	}
	w, h := int(bm.BmWidth), int(bm.BmHeight)
	if w <= 0 || h <= 0 || w > 256 || h > 256 {
		return nil
	}

	hdc, _, _ := procIconCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return nil
	}
	defer procIconDeleteDC.Call(hdc)

	bmi := winBitmapInfoHeader{
		BiSize:        uint32(unsafe.Sizeof(winBitmapInfoHeader{})),
		BiWidth:       int32(w),
		BiHeight:      -int32(h), // top-down
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: biRGB,
	}
	colorBits := make([]byte, w*h*4)
	lines, _, _ := procIconGetDIBits.Call(
		hdc,
		ii.HbmColor,
		0,
		uintptr(h),
		uintptr(unsafe.Pointer(&colorBits[0])),
		uintptr(unsafe.Pointer(&bmi)),
		dibRGBColors,
	)
	if lines == 0 {
		return nil
	}

	hasAlpha := false
	for i := 3; i < len(colorBits); i += 4 {
		if colorBits[i] != 0 {
			hasAlpha = true
			break
		}
	}

	var maskBits []byte
	if !hasAlpha && ii.HbmMask != 0 {
		maskBits = make([]byte, w*h*4)
		_, _, _ = procIconGetDIBits.Call(
			hdc,
			ii.HbmMask,
			0,
			uintptr(h),
			uintptr(unsafe.Pointer(&maskBits[0])),
			uintptr(unsafe.Pointer(&bmi)),
			dibRGBColors,
		)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		off := i * 4
		b := colorBits[off]
		g := colorBits[off+1]
		rv := colorBits[off+2]
		a := colorBits[off+3]
		if !hasAlpha {
			if len(maskBits) == len(colorBits) && maskBits[off] == 0 {
				a = 255
			} else if len(maskBits) == 0 && (rv != 0 || g != 0 || b != 0) {
				a = 255
			}
		}
		img.Pix[off] = rv
		img.Pix[off+1] = g
		img.Pix[off+2] = b
		img.Pix[off+3] = a
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil
	}
	return out.Bytes()
}
