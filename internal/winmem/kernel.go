package winmem

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	systemModuleInformation  = 11
	systemPoolTagInformation = 0x16
	objectTypesInformation   = 3
	statusInfoLengthMismatch = 0xC0000004
)

var (
	procNtQueryObject = ntdll.NewProc("NtQueryObject")
)

type systemPoolTag struct {
	Tag            [4]byte
	PagedAllocs    uint32
	PagedFrees     uint32
	PagedUsed      uintptr
	NonPagedAllocs uint32
	NonPagedFrees  uint32
	NonPagedUsed   uintptr
}

// PoolTag summarizes kernel pool allocations for one 4-byte pool tag.
type PoolTag struct {
	Tag            string
	PagedAllocs    uint32
	PagedFrees     uint32
	PagedUsed      uint64
	NonPagedAllocs uint32
	NonPagedFrees  uint32
	NonPagedUsed   uint64
}

// PagedActive returns net active paged allocations (Allocs - Frees).
func (p PoolTag) PagedActive() int64 {
	return int64(p.PagedAllocs) - int64(p.PagedFrees)
}

// NonPagedActive returns net active nonpaged allocations (Allocs - Frees).
func (p PoolTag) NonPagedActive() int64 {
	return int64(p.NonPagedAllocs) - int64(p.NonPagedFrees)
}

// TotalUsed returns combined paged + nonpaged bytes held by this tag.
func (p PoolTag) TotalUsed() uint64 {
	return p.PagedUsed + p.NonPagedUsed
}

// ReadPoolTags queries kernel pool allocations per tag via SystemPoolTagInformation.
// Works on modern Windows without elevation; callers should handle errors gracefully.
func ReadPoolTags() ([]PoolTag, error) {
	size := 512 << 10
	for attempt := 0; attempt < 6; attempt++ {
		buf := make([]byte, size)
		var ret uint32
		status, _, _ := procNtQuerySystemInformation.Call(
			uintptr(systemPoolTagInformation),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			uintptr(unsafe.Pointer(&ret)),
		)
		if uint32(int32(status)) == statusInfoLengthMismatch {
			if int(ret) > size {
				size = int(ret) + (64 << 10)
			} else {
				size *= 2
			}
			continue
		}
		if int32(status) < 0 {
			return nil, fmt.Errorf("NtQuerySystemInformation(PoolTag): NTSTATUS 0x%08X", uint32(int32(status)))
		}
		return parsePoolTags(buf, int(ret)), nil
	}
	return nil, fmt.Errorf("NtQuerySystemInformation(PoolTag): buffer exhausted")
}

func parsePoolTags(buf []byte, retLen int) []PoolTag {
	if len(buf) < int(unsafe.Sizeof(uintptr(0))) {
		return nil
	}
	count := int(*(*uint32)(unsafe.Pointer(&buf[0])))
	headerSize := int(unsafe.Sizeof(uintptr(0)))
	itemSize := int(unsafe.Sizeof(systemPoolTag{}))
	if retLen > 0 && retLen < len(buf) {
		buf = buf[:retLen]
	}
	maxCount := (len(buf) - headerSize) / itemSize
	if count > maxCount {
		count = maxCount
	}
	if count <= 0 {
		return nil
	}

	out := make([]PoolTag, 0, count)
	for i := 0; i < count; i++ {
		off := headerSize + i*itemSize
		raw := (*systemPoolTag)(unsafe.Pointer(&buf[off]))
		var ascii [4]byte
		for j := 0; j < 4; j++ {
			b := raw.Tag[j]
			if b >= 32 && b < 127 {
				ascii[j] = b
			} else {
				ascii[j] = '?'
			}
		}
		out = append(out, PoolTag{
			Tag:            string(ascii[:]),
			PagedAllocs:    raw.PagedAllocs,
			PagedFrees:     raw.PagedFrees,
			PagedUsed:      uint64(raw.PagedUsed),
			NonPagedAllocs: raw.NonPagedAllocs,
			NonPagedFrees:  raw.NonPagedFrees,
			NonPagedUsed:   uint64(raw.NonPagedUsed),
		})
	}
	return out
}

type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        uintptr
}

type objectTypeInfo struct {
	TypeName               unicodeString
	TotalNumberOfObjects   uint32
	TotalNumberOfHandles   uint32
	TotalPagedPoolUsage    uint32
	TotalNonPagedPoolUsage uint32
	TotalNamePoolUsage     uint32
	TotalHandleTableUsage  uint32
	HighWaterNumberObjects uint32
	HighWaterNumberHandles uint32
}

// ObjectTypeStat holds global kernel object and handle counts for one object type.
type ObjectTypeStat struct {
	Name             string
	Objects          uint32
	Handles          uint32
	HighWaterObjects uint32
	HighWaterHandles uint32
}

// ReadObjectTypes queries global kernel object counts via NtQueryObject(ObjectTypesInformation).
// Comparing Objects vs Handles exposes kernel reference leaks (e.g. zombie EPROCESS objects).
func ReadObjectTypes() ([]ObjectTypeStat, error) {
	size := 128 << 10
	for attempt := 0; attempt < 6; attempt++ {
		buf := make([]byte, size)
		var ret uint32
		status, _, _ := procNtQueryObject.Call(
			0,
			uintptr(objectTypesInformation),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			uintptr(unsafe.Pointer(&ret)),
		)
		if uint32(int32(status)) == statusInfoLengthMismatch {
			if int(ret) > size {
				size = int(ret) + (32 << 10)
			} else {
				size *= 2
			}
			continue
		}
		if int32(status) < 0 {
			return nil, fmt.Errorf("NtQueryObject(ObjectTypesInformation): NTSTATUS 0x%08X", uint32(int32(status)))
		}
		return parseObjectTypes(buf, int(ret)), nil
	}
	return nil, fmt.Errorf("NtQueryObject(ObjectTypesInformation): buffer exhausted")
}

func parseObjectTypes(buf []byte, retLen int) []ObjectTypeStat {
	if retLen > 0 && retLen < len(buf) {
		buf = buf[:retLen]
	}
	ptrSize := int(unsafe.Sizeof(uintptr(0)))
	if len(buf) < ptrSize {
		return nil
	}
	count := int(*(*uint32)(unsafe.Pointer(&buf[0])))
	base := uintptr(unsafe.Pointer(&buf[0]))
	off := ptrSize
	alignMask := ptrSize - 1
	minInfoSize := int(unsafe.Sizeof(objectTypeInfo{}))

	out := make([]ObjectTypeStat, 0, count)
	for i := 0; i < count && off+minInfoSize <= len(buf); i++ {
		info := (*objectTypeInfo)(unsafe.Pointer(&buf[off]))
		strPtr := info.TypeName.Buffer
		if strPtr < base {
			break
		}
		strOff := int(strPtr - base)
		maxLen := int(info.TypeName.MaximumLength)
		if strOff < off || strOff+maxLen > len(buf) {
			break
		}
		var name string
		if n := int(info.TypeName.Length / 2); n > 0 && strOff+n*2 <= len(buf) {
			u16 := make([]uint16, n)
			for j := 0; j < n; j++ {
				p := strOff + j*2
				u16[j] = uint16(buf[p]) | uint16(buf[p+1])<<8
			}
			name = syscall.UTF16ToString(u16)
		}
		out = append(out, ObjectTypeStat{
			Name:             name,
			Objects:          info.TotalNumberOfObjects,
			Handles:          info.TotalNumberOfHandles,
			HighWaterObjects: info.HighWaterNumberObjects,
			HighWaterHandles: info.HighWaterNumberHandles,
		})
		next := (strOff + maxLen + alignMask) &^ alignMask
		if next <= off {
			break
		}
		off = next
	}
	return out
}

type rtlProcessModuleInfo struct {
	Section          uintptr
	MappedBase       uintptr
	ImageBase        uintptr
	ImageSize        uint32
	Flags            uint32
	LoadOrderIndex   uint16
	InitOrderIndex   uint16
	LoadCount        uint16
	OffsetToFileName uint16
	FullPathName     [256]byte
}

// DriverModule represents one loaded kernel module.
type DriverModule struct {
	Name string
	Path string
}

// ListDrivers enumerates currently loaded kernel modules via
// NtQuerySystemInformation(SystemModuleInformation), which provides full module
// paths without requiring elevated privileges (unlike EnumDeviceDrivers whose
// base addresses are zeroed at medium IL on Windows 11).
func ListDrivers() ([]DriverModule, error) {
	size := 256 << 10
	for attempt := 0; attempt < 6; attempt++ {
		buf := make([]byte, size)
		var ret uint32
		status, _, _ := procNtQuerySystemInformation.Call(
			uintptr(systemModuleInformation),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			uintptr(unsafe.Pointer(&ret)),
		)
		if uint32(int32(status)) == statusInfoLengthMismatch {
			if int(ret) > size {
				size = int(ret) + (32 << 10)
			} else {
				size *= 2
			}
			continue
		}
		if int32(status) < 0 {
			return nil, fmt.Errorf("NtQuerySystemInformation(SystemModuleInformation): NTSTATUS 0x%08X", uint32(int32(status)))
		}
		return parseSystemModules(buf, int(ret)), nil
	}
	return nil, fmt.Errorf("NtQuerySystemInformation(SystemModuleInformation): buffer exhausted")
}

func parseSystemModules(buf []byte, retLen int) []DriverModule {
	if retLen > 0 && retLen < len(buf) {
		buf = buf[:retLen]
	}
	headerSize := int(unsafe.Sizeof(uintptr(0)))
	itemSize := int(unsafe.Sizeof(rtlProcessModuleInfo{}))
	if len(buf) < headerSize {
		return nil
	}
	count := int(*(*uint32)(unsafe.Pointer(&buf[0])))
	maxCount := (len(buf) - headerSize) / itemSize
	if count > maxCount {
		count = maxCount
	}
	if count <= 0 {
		return nil
	}

	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	seen := make(map[string]struct{}, count)
	out := make([]DriverModule, 0, count)
	for i := 0; i < count; i++ {
		off := headerSize + i*itemSize
		m := (*rtlProcessModuleInfo)(unsafe.Pointer(&buf[off]))
		raw := cString(m.FullPathName[:])
		if raw == "" {
			continue
		}
		resolved := resolveDriverPath(raw, sysRoot)
		key := strings.ToLower(resolved)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, DriverModule{
			Name: filepath.Base(resolved),
			Path: resolved,
		})
	}
	return out
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func resolveDriverPath(raw, sysRoot string) string {
	switch {
	case strings.HasPrefix(raw, `\??\`):
		return raw[4:]
	case len(raw) >= len(`\SystemRoot\`) && strings.EqualFold(raw[:len(`\SystemRoot\`)], `\SystemRoot\`):
		return filepath.Join(sysRoot, raw[len(`\SystemRoot\`):])
	case len(raw) >= len(`\System32\`) && strings.EqualFold(raw[:len(`\System32\`)], `\System32\`):
		return filepath.Join(sysRoot, raw[1:])
	case strings.HasPrefix(raw, `\`) && filepath.VolumeName(sysRoot) != "":
		return filepath.VolumeName(sysRoot) + raw
	default:
		return raw
	}
}

// MatchPoolTagsToDrivers searches loaded driver binaries on disk for the given
// 4-byte pool tags (both ASCII and little-endian immediate order).
// Intended for on-demand deep diagnosis (status --deep / doctor), never tight loops.
func MatchPoolTagsToDrivers(tags []string, drivers []DriverModule) map[string][]string {
	type pattern struct {
		tag string
		fwd []byte
		rev []byte
	}
	pats := make([]pattern, 0, len(tags))
	for _, t := range tags {
		if len(t) != 4 {
			continue
		}
		b := []byte(t)
		pats = append(pats, pattern{
			tag: t,
			fwd: b,
			rev: []byte{b[3], b[2], b[1], b[0]},
		})
	}
	hits := make(map[string][]string, len(pats))
	if len(pats) == 0 {
		return hits
	}

	const maxDriverFileBytes = 80 << 20
	for _, d := range drivers {
		if isCoreKernelImage(d.Name) {
			continue
		}
		info, err := os.Stat(d.Path)
		if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxDriverFileBytes {
			continue
		}
		data, err := os.ReadFile(d.Path)
		if err != nil {
			continue
		}
		for _, p := range pats {
			if bytes.Contains(data, p.fwd) || bytes.Contains(data, p.rev) {
				hits[p.tag] = append(hits[p.tag], d.Name)
			}
		}
	}
	return hits
}

func isCoreKernelImage(name string) bool {
	switch strings.ToLower(name) {
	case "ntoskrnl.exe", "hal.dll", "win32k.sys", "win32kbase.sys", "win32kfull.sys":
		return true
	default:
		return false
	}
}

// DescribePoolTag returns a short Chinese description for well-known Windows pool tags.
func DescribePoolTag(tag string) string {
	switch tag {
	case "Proc":
		return "内核进程对象 (EPROCESS)"
	case "MiP2":
		return "进程内存管理结构 (MM)"
	case "PsIn":
		return "进程镜像信息 (Ps)"
	case "Toke":
		return "访问令牌对象 (Token)"
	case "SeTl", "SeTd", "SeDt", "SeAt":
		return "安全令牌/描述符"
	case "Even":
		return "内核事件对象 (Event)"
	case "File":
		return "文件对象 (File)"
	case "Job ":
		return "作业对象 (Job)"
	case "Thre":
		return "内核线程对象 (ETHREAD)"
	case "Wnf ":
		return "Windows 通知状态 (WNF)"
	case "MmSt":
		return "内存映射区段控制区 (Section)"
	case "Cont":
		return "连续物理内存分配 (DMA/驱动)"
	case "EtwB":
		return "ETW 事件追踪缓冲"
	case "DxgK":
		return "DirectX 图形内核"
	case "FMfn", "FMsl":
		return "文件系统微过滤驱动框架 (FltMgr)"
	case "NtfF", "Ntff", "NtfE", "Ntfo":
		return "NTFS 文件系统"
	default:
		return ""
	}
}
