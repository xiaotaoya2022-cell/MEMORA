package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	advapi32DLL             = syscall.NewLazyDLL("advapi32.dll")
	procRegSetValueExW      = advapi32DLL.NewProc("RegSetValueExW")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
)

func cmdPath(args []string) {
	fs := flag.NewFlagSet("path", flag.ExitOnError)
	addDir := fs.String("add-user", "", "append directory to HKCU\\Environment Path")
	removeDir := fs.String("remove-user", "", "remove directory from HKCU\\Environment Path")
	_ = fs.Parse(args)

	var target string
	var add bool
	switch {
	case strings.TrimSpace(*addDir) != "" && strings.TrimSpace(*removeDir) != "":
		fmt.Fprintln(os.Stderr, "path: specify either --add-user or --remove-user, not both")
		os.Exit(2)
	case strings.TrimSpace(*addDir) != "":
		target = *addDir
		add = true
	case strings.TrimSpace(*removeDir) != "":
		target = *removeDir
		add = false
	default:
		fmt.Fprintln(os.Stderr, "用法: memora path --add-user <dir> | --remove-user <dir>")
		os.Exit(2)
	}

	if err := applyUserPathUpdate(target, add); err != nil {
		fmt.Fprintf(os.Stderr, "path update failed: %v\n", err)
		os.Exit(1)
	}
}

func normalizePathEntry(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"`)
	if s == "" {
		return ""
	}
	cleaned := filepath.Clean(s)
	if len(cleaned) > 3 && strings.HasSuffix(cleaned, `\`) {
		cleaned = strings.TrimRight(cleaned, `\`)
	} else if !strings.HasSuffix(cleaned, `:\`) {
		cleaned = strings.TrimRight(cleaned, `\`)
	}
	return cleaned
}

func updateUserPathString(currentPath, targetDir string, add bool) (string, bool) {
	normTarget := normalizePathEntry(targetDir)
	if normTarget == "" {
		return strings.TrimSpace(currentPath), false
	}

	rawParts := strings.Split(currentPath, ";")
	kept := make([]string, 0, len(rawParts)+1)
	hadTarget := false
	for _, part := range rawParts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		normPart := normalizePathEntry(trimmed)
		if normPart != "" && strings.EqualFold(normPart, normTarget) {
			hadTarget = true
			continue
		}
		kept = append(kept, trimmed)
	}

	if add {
		kept = append(kept, normTarget)
		joined := strings.Join(kept, ";")
		if hadTarget && strings.EqualFold(strings.TrimSpace(currentPath), joined) {
			return joined, false
		}
		return joined, !hadTarget || !strings.EqualFold(strings.TrimSpace(currentPath), joined)
	}

	joined := strings.Join(kept, ";")
	return joined, hadTarget
}

func applyUserPathUpdate(targetDir string, add bool) error {
	subKeyPtr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return err
	}
	const keyQuerySet = 0x0001 | 0x0002 // KEY_QUERY_VALUE | KEY_SET_VALUE
	var hKey syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, subKeyPtr, 0, keyQuerySet, &hKey); err != nil {
		return fmt.Errorf("RegOpenKeyEx HKCU\\Environment: %w", err)
	}
	defer syscall.RegCloseKey(hKey)

	valNamePtr, err := syscall.UTF16PtrFromString("Path")
	if err != nil {
		return err
	}

	var curPath string
	valType := uint32(syscall.REG_EXPAND_SZ)
	var bufLen uint32
	err = syscall.RegQueryValueEx(hKey, valNamePtr, nil, &valType, nil, &bufLen)
	if err == nil && bufLen >= 2 {
		buf := make([]byte, bufLen)
		var qType uint32
		if err := syscall.RegQueryValueEx(hKey, valNamePtr, nil, &qType, &buf[0], &bufLen); err == nil {
			if qType == syscall.REG_SZ || qType == syscall.REG_EXPAND_SZ {
				valType = qType
			}
			u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), int(bufLen)/2)
			curPath = syscall.UTF16ToString(u16)
		}
	} else if err != nil && err != syscall.ERROR_FILE_NOT_FOUND {
		return fmt.Errorf("RegQueryValueEx Path: %w", err)
	}

	if valType != syscall.REG_SZ && valType != syscall.REG_EXPAND_SZ {
		valType = syscall.REG_EXPAND_SZ
	}

	newPath, changed := updateUserPathString(curPath, targetDir, add)
	if !changed {
		return nil
	}

	utf16Val, err := syscall.UTF16FromString(newPath)
	if err != nil {
		return err
	}
	cbData := uint32(len(utf16Val) * 2)
	r0, _, e1 := procRegSetValueExW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valNamePtr)),
		0,
		uintptr(valType),
		uintptr(unsafe.Pointer(&utf16Val[0])),
		uintptr(cbData),
	)
	if r0 != 0 {
		if e1 != nil && e1 != syscall.Errno(0) {
			return fmt.Errorf("RegSetValueExW Path: %w", e1)
		}
		return fmt.Errorf("RegSetValueExW Path failed with code %d", r0)
	}

	envPtr, _ := syscall.UTF16PtrFromString("Environment")
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	var result uintptr
	_, _, _ = procSendMessageTimeoutW.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(smtoAbortIfHung),
		1000,
		uintptr(unsafe.Pointer(&result)),
	)
	return nil
}
