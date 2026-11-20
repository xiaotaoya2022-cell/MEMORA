package main

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
	maxProcIconCache               = 768
	maxNamePathCache               = 1024

	wmGetIcon       = 0x007F
	wmIconSmall     = 0
	wmIconBig       = 1
	wmIconSmall2    = 2
	smtoAbortIfHung = 0x0002
	gclpHIcon       = ^uintptr(13) // -14 (GCLP_HICON)
	gclpHIconSm     = ^uintptr(33) // -34 (GCLP_HICONSM)
)

var (
	iconShell32 = syscall.NewLazyDLL("shell32.dll")
	iconUser32  = syscall.NewLazyDLL("user32.dll")
	iconGdi32   = syscall.NewLazyDLL("gdi32.dll")
	iconKernel  = syscall.NewLazyDLL("kernel32.dll")

	procSHGetFileInfoW            = iconShell32.NewProc("SHGetFileInfoW")
	procExtractIconExW            = iconShell32.NewProc("ExtractIconExW")
	procPrivateExtractIconsW      = iconUser32.NewProc("PrivateExtractIconsW")
	procIconGetIconInfo           = iconUser32.NewProc("GetIconInfo")
	procIconDestroyIcon           = iconUser32.NewProc("DestroyIcon")
	procIconEnumWindows           = iconUser32.NewProc("EnumWindows")
	procIconIsWindowVisible       = iconUser32.NewProc("IsWindowVisible")
	procIconGetWindowThreadPID    = iconUser32.NewProc("GetWindowThreadProcessId")
	procIconSendMessageTimeoutW   = iconUser32.NewProc("SendMessageTimeoutW")
	procIconGetClassLongPtrW      = iconUser32.NewProc("GetClassLongPtrW")
	procIconCreateCompatibleDC    = iconGdi32.NewProc("CreateCompatibleDC")
	procIconDeleteDC              = iconGdi32.NewProc("DeleteDC")
	procIconGetObjectW            = iconGdi32.NewProc("GetObjectW")
	procIconGetDIBits             = iconGdi32.NewProc("GetDIBits")
	procIconDeleteObject          = iconGdi32.NewProc("DeleteObject")
	procIconOpenProcess           = iconKernel.NewProc("OpenProcess")
	procIconQueryFullProcessImage = iconKernel.NewProc("QueryFullProcessImageNameW")
	procIconCloseHandle           = iconKernel.NewProc("CloseHandle")

	procIconMu        sync.RWMutex
	procIconCache     = map[string][]byte{}
	procNamePathCache = map[string]string{}
	procPIDPathCache  = map[uint32]string{}

	enumWinOnce sync.Once
	enumWinCB   uintptr
	enumWinMu   sync.Mutex
	enumWinPID  uint32
	enumWinHWND uintptr
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

	pngBytes := resolveProcessIconPNG(pid, name)
	if len(pngBytes) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(pngBytes)
}

func getCachedIcon(key string) ([]byte, bool) {
	if key == "" {
		return nil, false
	}
	procIconMu.RLock()
	v, ok := procIconCache[key]
	procIconMu.RUnlock()
	return v, ok
}

func putCachedIcon(key string, data []byte) {
	if key == "" {
		return
	}
	procIconMu.Lock()
	if len(procIconCache) >= maxProcIconCache {
		procIconCache = map[string][]byte{}
	}
	procIconCache[key] = data
	procIconMu.Unlock()
}

func rememberNamePath(name, exePath string) {
	if name == "" || exePath == "" {
		return
	}
	low := strings.ToLower(strings.TrimSpace(name))
	procIconMu.Lock()
	if len(procNamePathCache) >= maxNamePathCache {
		procNamePathCache = map[string]string{}
	}
	procNamePathCache[low] = exePath
	procIconMu.Unlock()
}

func lookupNamePath(name string) string {
	if name == "" {
		return ""
	}
	procIconMu.RLock()
	p := procNamePathCache[strings.ToLower(strings.TrimSpace(name))]
	procIconMu.RUnlock()
	return p
}

func resolveProcessIconPNG(pid uint32, name string) []byte {
	var procs []proc.Info
	procsLoaded := false
	getProcs := func() []proc.Info {
		if !procsLoaded {
			procs, _, _ = proc.ListAll()
			procsLoaded = true
		}
		return procs
	}

	exePath := queryProcessExePath(pid)
	if exePath != "" && name != "" {
		rememberNamePath(name, exePath)
	}
	if exePath == "" && name != "" {
		for _, p := range getProcs() {
			if strings.EqualFold(p.Name, name) {
				if pth := queryProcessExePath(p.PID); pth != "" {
					exePath = pth
					if pid == 0 {
						pid = p.PID
					}
					rememberNamePath(name, exePath)
					break
				}
			}
		}
	}
	if exePath == "" && name != "" {
		exePath = lookupNamePath(name)
	}

	baseName := name
	if baseName == "" && exePath != "" {
		baseName = filepath.Base(exePath)
	}

	// Fast path: if this executable already has a resolved icon cached under its own path, return immediately.
	if exePath != "" {
		if cached, ok := getCachedIcon("self:" + strings.ToLower(exePath)); ok && len(cached) > 0 {
			return cached
		}
	}

	// 1. Self-owned embedded PE icon (PrivateExtractIconsW / ExtractIconExW) — never returns generic blank .exe icon.
	if exePath != "" {
		if pngBytes := extractEmbeddedPEIconPNG(exePath); len(pngBytes) > 0 {
			putCachedIcon("self:"+strings.ToLower(exePath), pngBytes)
			if baseName != "" {
				putCachedIcon("name:"+strings.ToLower(baseName), pngBytes)
			}
			return pngBytes
		}
		// 2. Packaged / UWP AppxManifest PNG logo.
		if pngBytes := extractAppxLogoPNG(exePath); len(pngBytes) > 0 {
			putCachedIcon("self:"+strings.ToLower(exePath), pngBytes)
			if baseName != "" {
				putCachedIcon("name:"+strings.ToLower(baseName), pngBytes)
			}
			return pngBytes
		}
	}

	// 3. Runtime top-level window icon owned by this PID (covers GUI/Electron/CEF processes without embedded .rc icons).
	if pid > 4 {
		if pngBytes := extractWindowIconPNG(pid); len(pngBytes) > 0 {
			if baseName != "" {
				putCachedIcon("name:"+strings.ToLower(baseName), pngBytes)
			}
			return pngBytes
		}
	}

	// 4. Ancestor process tree inheritance (e.g., WeChatAppEx.exe -> Weixin.exe, crashpad_handler.exe -> host app).
	if pid > 4 {
		if parentIcon := resolveParentTreeIconPNG(pid, exePath, baseName, getProcs()); len(parentIcon) > 0 {
			if baseName != "" && !isSharedSubhostBinary(baseName) {
				putCachedIcon("name:"+strings.ToLower(baseName), parentIcon)
			}
			return parentIcon
		}
	}

	// Check name-level resolved cache (for exited/pid=0 lookups of known app names).
	if baseName != "" {
		if cached, ok := getCachedIcon("name:" + strings.ToLower(baseName)); ok && len(cached) > 0 {
			return cached
		}
	}

	// 5. Sibling / ancestor directory primary executable scan + known WeChat/Weixin helper mapping.
	if exePath != "" {
		dirKey := "dir:" + strings.ToLower(exePath)
		if cached, ok := getCachedIcon(dirKey); ok {
			if len(cached) > 0 {
				return cached
			}
		} else {
			if pngBytes := findAncestorDirAppIconPNG(exePath); len(pngBytes) > 0 {
				putCachedIcon(dirKey, pngBytes)
				if baseName != "" {
					putCachedIcon("name:"+strings.ToLower(baseName), pngBytes)
				}
				return pngBytes
			}
			putCachedIcon(dirKey, nil)
		}
	}
	if knownBytes := findWellKnownHelperAppIconPNG(baseName); len(knownBytes) > 0 {
		if baseName != "" {
			putCachedIcon("name:"+strings.ToLower(baseName), knownBytes)
		}
		return knownBytes
	}

	// 6. Last resort: shell icon (for system host binaries such as svchost.exe / services.exe).
	if exePath != "" {
		shKey := "shfi:" + strings.ToLower(exePath)
		if cached, ok := getCachedIcon(shKey); ok {
			return cached
		}
		pngBytes := extractShellFileIconPNG(exePath)
		putCachedIcon(shKey, pngBytes)
		return pngBytes
	}
	return nil
}

func isSharedSubhostBinary(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "crashpad_handler.exe", "msedgewebview2.exe", "conhost.exe", "node.exe", "python.exe", "pythonw.exe":
		return true
	default:
		return false
	}
}

func queryProcessExePath(pid uint32) string {
	if pid <= 4 {
		return ""
	}
	procIconMu.RLock()
	cached, ok := procPIDPathCache[pid]
	procIconMu.RUnlock()
	if ok {
		return cached
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
	path := syscall.UTF16ToString(buf[:size])
	procIconMu.Lock()
	if len(procPIDPathCache) >= maxNamePathCache {
		procPIDPathCache = map[uint32]string{}
	}
	procPIDPathCache[pid] = path
	procIconMu.Unlock()
	return path
}

func extractEmbeddedPEIconPNG(exePath string) []byte {
	pathPtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return nil
	}
	for _, px := range []int32{48, 32} {
		var hIcon uintptr
		var iconID uint32
		cnt, _, _ := procPrivateExtractIconsW.Call(
			uintptr(unsafe.Pointer(pathPtr)),
			0,
			uintptr(px),
			uintptr(px),
			uintptr(unsafe.Pointer(&hIcon)),
			uintptr(unsafe.Pointer(&iconID)),
			1,
			0,
		)
		if cnt > 0 && cnt != 0xFFFFFFFF && hIcon != 0 {
			if pngBytes := hiconToPNG(hIcon, true); len(pngBytes) > 0 {
				return pngBytes
			}
		}
	}

	var hLarge uintptr
	cnt, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&hLarge)),
		0,
		1,
	)
	if cnt > 0 && cnt != 0xFFFFFFFF && hLarge != 0 {
		return hiconToPNG(hLarge, true)
	}
	return nil
}

func extractShellFileIconPNG(exePath string) []byte {
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
	return hiconToPNG(shfi.HIcon, true)
}

func findTopWindowForPID(pid uint32) uintptr {
	if pid <= 4 {
		return 0
	}
	enumWinOnce.Do(func() {
		enumWinCB = syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
			vis, _, _ := procIconIsWindowVisible.Call(hwnd)
			if vis == 0 {
				return 1
			}
			var winPID uint32
			_, _, _ = procIconGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&winPID)))
			if winPID == enumWinPID {
				enumWinHWND = hwnd
				return 0
			}
			return 1
		})
	})

	enumWinMu.Lock()
	defer enumWinMu.Unlock()
	enumWinPID = pid
	enumWinHWND = 0
	_, _, _ = procIconEnumWindows.Call(enumWinCB, 0)
	return enumWinHWND
}

func extractWindowIconPNG(pid uint32) []byte {
	hwnd := findTopWindowForPID(pid)
	if hwnd == 0 {
		return nil
	}
	for _, wParam := range []uintptr{wmIconBig, wmIconSmall2, wmIconSmall} {
		var hIcon uintptr
		ok, _, _ := procIconSendMessageTimeoutW.Call(
			hwnd,
			wmGetIcon,
			wParam,
			0,
			smtoAbortIfHung,
			150,
			uintptr(unsafe.Pointer(&hIcon)),
		)
		if ok != 0 && hIcon != 0 {
			if pngBytes := hiconToPNG(hIcon, false); len(pngBytes) > 0 {
				return pngBytes
			}
		}
	}
	for _, idx := range []uintptr{gclpHIcon, gclpHIconSm} {
		hIcon, _, _ := procIconGetClassLongPtrW.Call(hwnd, idx)
		if hIcon != 0 {
			if pngBytes := hiconToPNG(hIcon, false); len(pngBytes) > 0 {
				return pngBytes
			}
		}
	}
	return nil
}

func resolveParentTreeIconPNG(pid uint32, childExe, childName string, procs []proc.Info) []byte {
	if len(procs) == 0 {
		return nil
	}
	byPID := make(map[uint32]proc.Info, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}

	curPID := pid
	for depth := 0; depth < 5; depth++ {
		cur, ok := byPID[curPID]
		if !ok || cur.ParentPID <= 4 || cur.ParentPID == curPID {
			break
		}
		parent, ok := byPID[cur.ParentPID]
		if !ok {
			break
		}
		curPID = parent.PID
		if isGenericOSShell(parent.Name) {
			break
		}
		parentExe := queryProcessExePath(parent.PID)
		if parentExe == "" {
			continue
		}
		if !shouldInheritParentIcon(childExe, childName, parentExe, parent.Name) {
			continue
		}
		if cached, ok := getCachedIcon("self:" + strings.ToLower(parentExe)); ok && len(cached) > 0 {
			return cached
		}
		if pngBytes := extractEmbeddedPEIconPNG(parentExe); len(pngBytes) > 0 {
			putCachedIcon("self:"+strings.ToLower(parentExe), pngBytes)
			return pngBytes
		}
		if pngBytes := extractAppxLogoPNG(parentExe); len(pngBytes) > 0 {
			putCachedIcon("self:"+strings.ToLower(parentExe), pngBytes)
			return pngBytes
		}
		if pngBytes := extractWindowIconPNG(parent.PID); len(pngBytes) > 0 {
			return pngBytes
		}
	}
	return nil
}

func isGenericOSShell(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "system", "idle", "smss.exe", "csrss.exe", "wininit.exe", "services.exe",
		"lsass.exe", "winlogon.exe", "sihost.exe", "userinit.exe", "explorer.exe",
		"svchost.exe", "taskmgr.exe", "runtimebroker.exe", "cmd.exe", "powershell.exe",
		"pwsh.exe", "windowsterminal.exe", "conhost.exe", "wsl.exe":
		return true
	default:
		return false
	}
}

func isKnownAppHelperName(name string) bool {
	low := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), ".exe"))
	if low == "" {
		return false
	}
	switch {
	case strings.HasPrefix(low, "wechatappex"),
		strings.HasPrefix(low, "wechatutility"),
		strings.HasPrefix(low, "wechatweb"),
		strings.HasPrefix(low, "weixin"),
		strings.HasPrefix(low, "msedgewebview2"),
		strings.HasPrefix(low, "webview2"),
		strings.HasPrefix(low, "crashpad_handler"),
		strings.HasPrefix(low, "language_server"),
		strings.HasPrefix(low, "qtwebengineprocess"),
		strings.HasPrefix(low, "cef"),
		strings.HasPrefix(low, "wetype_"),
		strings.HasSuffix(low, "helper"),
		strings.HasSuffix(low, "renderer"),
		strings.HasSuffix(low, "_guard"):
		return true
	default:
		return false
	}
}

func shouldInheritParentIcon(childExe, childName, parentExe, parentName string) bool {
	if isKnownAppHelperName(childName) {
		return true
	}
	if childExe == "" || parentExe == "" {
		return false
	}
	cClean := strings.ToLower(filepath.Clean(filepath.Dir(childExe)))
	pClean := strings.ToLower(filepath.Clean(filepath.Dir(parentExe)))
	if cClean == pClean || strings.HasPrefix(cClean, pClean+`\`) || strings.HasPrefix(pClean, cClean+`\`) {
		// Ensure shared folder is actually inside an application root (at least 3 path segments, e.g. C:\Program Files\App).
		segs := strings.Split(strings.TrimRight(pClean, `\`), `\`)
		if len(segs) >= 3 && !isStopAppScanDir(pClean) {
			return true
		}
	}
	return false
}

func isStopAppScanDir(dir string) bool {
	clean := strings.ToLower(strings.TrimRight(filepath.Clean(dir), `\`))
	if len(clean) <= 2 || (len(clean) == 3 && clean[1] == ':') {
		return true
	}
	base := filepath.Base(clean)
	switch base {
	case "", "program files", "program files (x86)", "programdata", "windows", "system32",
		"syswow64", "systemapps", "windowsapps", "users", "appdata", "local", "roaming",
		"programs", "common files", "exeapp":
		return true
	}
	if strings.Contains(clean, `\windows\system32`) || strings.Contains(clean, `\windows\syswow64`) {
		return true
	}
	return false
}

func findAncestorDirAppIconPNG(exePath string) []byte {
	dir := filepath.Dir(exePath)
	for depth := 0; depth < 3; depth++ {
		if isStopAppScanDir(dir) {
			break
		}
		entries, err := os.ReadDir(dir)
		if err == nil {
			folderHint := strings.ToLower(filepath.Base(dir))
			var candidates []string
			for _, ent := range entries {
				if ent.IsDir() {
					continue
				}
				nm := ent.Name()
				low := strings.ToLower(nm)
				if !strings.HasSuffix(low, ".exe") {
					continue
				}
				if strings.Contains(low, "unins") || strings.Contains(low, "setup") ||
					strings.Contains(low, "crash") || strings.Contains(low, "update") ||
					strings.Contains(low, "report") || strings.Contains(low, "elevat") ||
					strings.EqualFold(filepath.Join(dir, nm), exePath) {
					continue
				}
				candidates = append(candidates, filepath.Join(dir, nm))
			}
			sort.Slice(candidates, func(i, j int) bool {
				si := strings.ToLower(strings.TrimSuffix(filepath.Base(candidates[i]), ".exe"))
				sj := strings.ToLower(strings.TrimSuffix(filepath.Base(candidates[j]), ".exe"))
				mi := strings.Contains(folderHint, si) || strings.Contains(si, folderHint)
				mj := strings.Contains(folderHint, sj) || strings.Contains(sj, folderHint)
				if mi != mj {
					return mi
				}
				return len(si) < len(sj)
			})
			for _, cand := range candidates {
				if cached, ok := getCachedIcon("self:" + strings.ToLower(cand)); ok && len(cached) > 0 {
					return cached
				}
				if pngBytes := extractEmbeddedPEIconPNG(cand); len(pngBytes) > 0 {
					putCachedIcon("self:"+strings.ToLower(cand), pngBytes)
					return pngBytes
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

func findWellKnownHelperAppIconPNG(name string) []byte {
	low := strings.ToLower(strings.TrimSpace(name))
	var candidates []string
	switch {
	case strings.HasPrefix(low, "wechat") || strings.HasPrefix(low, "weixin") || strings.HasPrefix(low, "xwechat"):
		pf := os.Getenv("ProgramFiles")
		pf86 := os.Getenv("ProgramFiles(x86)")
		local := os.Getenv("LOCALAPPDATA")
		candidates = []string{
			filepath.Join(pf, "Tencent", "Weixin", "Weixin.exe"),
			filepath.Join(pf, "Tencent", "WeChat", "WeChat.exe"),
			filepath.Join(pf86, "Tencent", "WeChat", "WeChat.exe"),
			filepath.Join(pf86, "Tencent", "Weixin", "Weixin.exe"),
			filepath.Join(local, "Tencent", "WeChat", "WeChat.exe"),
		}
	case strings.HasPrefix(low, "wetype"):
		pf := os.Getenv("ProgramFiles")
		pf86 := os.Getenv("ProgramFiles(x86)")
		candidates = []string{
			filepath.Join(pf, "Tencent", "WeType", "WeType.exe"),
			filepath.Join(pf86, "Tencent", "WeType", "WeType.exe"),
		}
	case strings.HasPrefix(low, "msedgewebview2"):
		pf := os.Getenv("ProgramFiles")
		pf86 := os.Getenv("ProgramFiles(x86)")
		candidates = []string{
			filepath.Join(pf86, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(pf, "Microsoft", "Edge", "Application", "msedge.exe"),
		}
	}
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		if _, err := os.Stat(cand); err != nil {
			continue
		}
		if cached, ok := getCachedIcon("self:" + strings.ToLower(cand)); ok && len(cached) > 0 {
			return cached
		}
		if pngBytes := extractEmbeddedPEIconPNG(cand); len(pngBytes) > 0 {
			putCachedIcon("self:"+strings.ToLower(cand), pngBytes)
			return pngBytes
		}
	}
	return nil
}

func extractAppxLogoPNG(exePath string) []byte {
	dir := filepath.Dir(exePath)
	var manifestDir string
	var manifestRaw []byte
	for i := 0; i < 2; i++ {
		if isStopAppScanDir(dir) {
			break
		}
		raw, err := os.ReadFile(filepath.Join(dir, "AppxManifest.xml"))
		if err == nil {
			manifestDir = dir
			manifestRaw = raw
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if len(manifestRaw) == 0 {
		return nil
	}

	rels := parseAppxManifestLogos(manifestRaw)
	for _, rel := range rels {
		imgPath := findBestAppxLogoFile(manifestDir, rel)
		if imgPath == "" {
			continue
		}
		raw, err := os.ReadFile(imgPath)
		if err != nil || len(raw) < 16 {
			continue
		}
		if _, err := png.DecodeConfig(bytes.NewReader(raw)); err == nil {
			return raw
		}
	}
	return nil
}

func parseAppxManifestLogos(raw []byte) []string {
	type visualAttr struct {
		Logo           string `xml:"Logo"`
		Square44       string `xml:"Square44x44Logo,attr"`
		Square150      string `xml:"Square150x150Logo,attr"`
		VisualLogoAttr string `xml:"Logo,attr"`
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var propsLogo string
	var appLogos []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "Logo":
			var txt string
			if dec.DecodeElement(&txt, &se) == nil && strings.TrimSpace(txt) != "" {
				propsLogo = strings.TrimSpace(txt)
			}
		case "VisualElements":
			var va visualAttr
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "Square44x44Logo":
					va.Square44 = strings.TrimSpace(a.Value)
				case "Square150x150Logo":
					va.Square150 = strings.TrimSpace(a.Value)
				case "Logo":
					va.VisualLogoAttr = strings.TrimSpace(a.Value)
				}
			}
			for _, v := range []string{va.Square44, va.Square150, va.VisualLogoAttr} {
				if v != "" {
					appLogos = append(appLogos, v)
				}
			}
		}
	}
	if propsLogo != "" {
		appLogos = append(appLogos, propsLogo)
	}
	return appLogos
}

func findBestAppxLogoFile(manifestDir, rel string) string {
	cleanRel := filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/"))
	direct := filepath.Join(manifestDir, cleanRel)
	if info, err := os.Stat(direct); err == nil && !info.IsDir() {
		return direct
	}
	ext := filepath.Ext(cleanRel)
	if ext == "" {
		ext = ".png"
	}
	stem := strings.TrimSuffix(filepath.Base(cleanRel), ext)
	assetDir := filepath.Dir(direct)
	entries, err := os.ReadDir(assetDir)
	if err != nil {
		return ""
	}
	lowStem := strings.ToLower(stem)
	var matches []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		nm := ent.Name()
		low := strings.ToLower(nm)
		if !strings.HasSuffix(low, ".png") || !strings.HasPrefix(low, lowStem) {
			continue
		}
		if strings.Contains(low, "contrast-black") || strings.Contains(low, "contrast-white") {
			continue
		}
		matches = append(matches, filepath.Join(assetDir, nm))
	}
	if len(matches) == 0 {
		return ""
	}
	score := func(p string) int {
		l := strings.ToLower(filepath.Base(p))
		s := 0
		switch {
		case strings.Contains(l, "targetsize-48"):
			s += 100
		case strings.Contains(l, "targetsize-32"):
			s += 95
		case strings.Contains(l, "targetsize-64"):
			s += 90
		case strings.Contains(l, "scale-200"):
			s += 80
		case strings.Contains(l, "scale-150"):
			s += 70
		case strings.Contains(l, "scale-100"):
			s += 60
		}
		if strings.Contains(l, "altform-unplated") {
			s += 10
		}
		return s
	}
	sort.Slice(matches, func(i, j int) bool {
		return score(matches[i]) > score(matches[j])
	})
	return matches[0]
}

func hiconToPNG(hIcon uintptr, destroyAfter bool) []byte {
	if hIcon == 0 {
		return nil
	}
	if destroyAfter {
		defer procIconDestroyIcon.Call(hIcon)
	}

	var ii winIconInfo
	ok, _, _ := procIconGetIconInfo.Call(hIcon, uintptr(unsafe.Pointer(&ii)))
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

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	nonTransparent := 0
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
		if a > 0 {
			nonTransparent++
		}
		img.Pix[off] = rv
		img.Pix[off+1] = g
		img.Pix[off+2] = b
		img.Pix[off+3] = a
	}
	if nonTransparent == 0 {
		return nil
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil
	}
	return out.Bytes()
}
