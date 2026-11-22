package winmem

import (
	"bytes"
	"debug/pe"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	versionDLL                 = syscall.NewLazyDLL("version.dll")
	procGetFileVersionInfoSize = versionDLL.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfo     = versionDLL.NewProc("GetFileVersionInfoW")
	procVerQueryValue          = versionDLL.NewProc("VerQueryValueW")
)

// ProcessHookDriver describes a loaded third-party kernel driver whose PE import
// table references process/object-hook kernel APIs capable of holding EPROCESS references.
type ProcessHookDriver struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Company    string   `json:"company,omitempty"`
	FileVer    string   `json:"file_version,omitempty"`
	Hooks      []string `json:"hooks"`
	RiskLevel  string   `json:"risk_level"` // "high" | "medium" | "low"
	Category   string   `json:"category"`   // "security_hook" | "vm_hook" | "wfp_hook" | "gpu_driver" | "other"
	PoolTag    string   `json:"pool_tag,omitempty"`
	PoolBytes  uint64   `json:"pool_bytes,omitempty"`
	PoolActive int64    `json:"pool_active,omitempty"`
}

var processHookSymbols = []string{
	"PsSetCreateProcessNotifyRoutineEx2",
	"PsSetCreateProcessNotifyRoutineEx",
	"PsSetCreateProcessNotifyRoutine",
	"ObRegisterCallbacks",
	"PsLookupProcessByProcessId",
	"ObfReferenceObject",
	"PsSetCreateThreadNotifyRoutine",
}

var (
	driverAuditMu    sync.Mutex
	driverAuditAt    time.Time
	driverAuditCache []ProcessHookDriver
)

// AuditProcessHookDrivers scans loaded non-Microsoft kernel drivers' PE import tables
// to identify which Ring-0 modules hook process creation/exit or reference EPROCESS objects.
// Results are cached for 5 minutes unless forceRescan is true.
func AuditProcessHookDrivers(drivers []DriverModule, forceRescan bool) []ProcessHookDriver {
	driverAuditMu.Lock()
	if !forceRescan && !driverAuditAt.IsZero() && time.Since(driverAuditAt) < 5*time.Minute && len(driverAuditCache) > 0 {
		out := make([]ProcessHookDriver, len(driverAuditCache))
		copy(out, driverAuditCache)
		driverAuditMu.Unlock()
		return out
	}
	driverAuditMu.Unlock()

	if len(drivers) == 0 {
		drivers, _ = ListDrivers()
	}

	const maxScanSize = 64 << 20
	var out []ProcessHookDriver
	for _, d := range drivers {
		if isCoreKernelImage(d.Name) {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(d.Name), ".sys") {
			continue
		}
		info, err := os.Stat(d.Path)
		if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxScanSize {
			continue
		}
		company, fileVer := readFileVersionCompany(d.Path)
		if isMicrosoftCompany(company) {
			continue
		}

		hooks := inspectDriverProcessHooks(d.Path)
		if len(hooks) == 0 {
			continue
		}
		cat, risk := classifyHookDriver(d.Name, company, hooks)
		out = append(out, ProcessHookDriver{
			Name:      d.Name,
			Path:      d.Path,
			Company:   company,
			FileVer:   fileVer,
			Hooks:     hooks,
			RiskLevel: risk,
			Category:  cat,
		})
	}

	sortHookDrivers(out)

	driverAuditMu.Lock()
	driverAuditCache = make([]ProcessHookDriver, len(out))
	copy(driverAuditCache, out)
	driverAuditAt = time.Now()
	driverAuditMu.Unlock()

	return out
}

func sortHookDrivers(list []ProcessHookDriver) {
	riskRank := func(r string) int {
		switch r {
		case "high":
			return 3
		case "medium":
			return 2
		default:
			return 1
		}
	}
	sort.Slice(list, func(i, j int) bool {
		ri, rj := riskRank(list[i].RiskLevel), riskRank(list[j].RiskLevel)
		if ri != rj {
			return ri > rj
		}
		if list[i].PoolActive != list[j].PoolActive {
			return list[i].PoolActive > list[j].PoolActive
		}
		if len(list[i].Hooks) != len(list[j].Hooks) {
			return len(list[i].Hooks) > len(list[j].Hooks)
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
}

func isMicrosoftCompany(company string) bool {
	low := strings.ToLower(strings.TrimSpace(company))
	return strings.Contains(low, "microsoft")
}

func classifyHookDriver(name, company string, hooks []string) (category, risk string) {
	lowName := strings.ToLower(filepath.Base(name))
	lowComp := strings.ToLower(company)

	// Known GPU display/platform drivers legitimately register process notify callbacks
	// for per-app graphics profiles; still report them transparently as low-risk GPU drivers.
	switch lowName {
	case "nvlddmkm.sys", "nvpcf.sys", "amdkmdag.sys", "amdfendr.sys", "igdkmd64.sys", "igdkmdn64.sys":
		return "gpu_driver", "low"
	}

	hasCreateProc := false
	hasObReg := false
	hasLookup := false
	for _, h := range hooks {
		switch {
		case strings.HasPrefix(h, "PsSetCreateProcessNotifyRoutine"):
			hasCreateProc = true
		case h == "ObRegisterCallbacks":
			hasObReg = true
		case h == "PsLookupProcessByProcessId":
			hasLookup = true
		}
	}

	// Virtualization / sandbox / hardening drivers (VirtualBox, VMware, MuMu, etc.)
	if strings.Contains(lowName, "vbox") || strings.Contains(lowName, "mumu") ||
		strings.Contains(lowName, "vmx") || strings.Contains(lowComp, "oracle") {
		if hasCreateProc || hasObReg {
			return "vm_hook", "high"
		}
		return "vm_hook", "medium"
	}

	// Security / AV / HIPS / anti-cheat kernel hooks
	if strings.Contains(lowName, "sysdiag") || strings.Contains(lowName, "lnvscenter") ||
		strings.Contains(lowName, "lrtp") || strings.Contains(lowName, "netrtp") ||
		strings.Contains(lowName, "360") || strings.Contains(lowName, "qqpc") ||
		strings.Contains(lowName, "edr") || strings.Contains(lowName, "av") ||
		strings.Contains(lowComp, "huorong") || strings.Contains(lowComp, "security") ||
		hasObReg {
		if hasCreateProc || hasObReg {
			return "security_hook", "high"
		}
		return "security_hook", "medium"
	}

	// WFP / network filter / VPN / accelerator drivers
	if strings.Contains(lowName, "wfp") || strings.Contains(lowName, "netfilter") ||
		strings.Contains(lowName, "wintun") || strings.Contains(lowName, "tap") ||
		strings.Contains(lowComp, "win 7 ddk") || strings.Contains(lowComp, "wireguard") {
		if hasCreateProc {
			return "wfp_hook", "medium"
		}
		if hasLookup {
			return "wfp_hook", "medium"
		}
		return "wfp_hook", "low"
	}

	if hasCreateProc {
		return "other", "high"
	}
	if hasLookup {
		return "other", "medium"
	}
	return "other", "low"
}

func inspectDriverProcessHooks(path string) []string {
	found := make(map[string]bool)

	// Primary: authoritative PE Import Table parsing via Go stdlib debug/pe.
	if f, err := pe.Open(path); err == nil {
		if syms, err := f.ImportedSymbols(); err == nil {
			for _, sym := range syms {
				// Format is "SymbolName:DLLName"
				name := sym
				if idx := strings.IndexByte(sym, ':'); idx > 0 {
					name = sym[:idx]
				}
				for _, target := range processHookSymbols {
					if name == target {
						found[normalizeHookName(target)] = true
					}
				}
			}
		}
		_ = f.Close()
	}

	// Secondary fallback: some kernel drivers resolve APIs dynamically via MmGetSystemRoutineAddress
	// (UTF-16LE or ASCII string literals) instead of static IAT entries.
	if len(found) == 0 {
		if data, err := os.ReadFile(path); err == nil {
			for _, target := range processHookSymbols {
				if bytes.Contains(data, []byte(target)) {
					found[normalizeHookName(target)] = true
				}
			}
		}
	}

	if len(found) == 0 {
		return nil
	}
	// Only return drivers that have at least one process/callback hook (ignore standalone ObfReferenceObject).
	if !found["PsSetCreateProcessNotifyRoutine"] && !found["ObRegisterCallbacks"] && !found["PsLookupProcessByProcessId"] {
		return nil
	}

	order := []string{
		"PsSetCreateProcessNotifyRoutine",
		"ObRegisterCallbacks",
		"PsLookupProcessByProcessId",
		"ObfReferenceObject",
		"PsSetCreateThreadNotifyRoutine",
	}
	var out []string
	for _, k := range order {
		if found[k] {
			out = append(out, k)
		}
	}
	return out
}

func normalizeHookName(sym string) string {
	if strings.HasPrefix(sym, "PsSetCreateProcessNotifyRoutine") {
		return "PsSetCreateProcessNotifyRoutine"
	}
	return sym
}

func readFileVersionCompany(path string) (company, fileVersion string) {
	if procGetFileVersionInfoSize.Find() != nil {
		return "", ""
	}
	pUTF16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", ""
	}
	var dummy uint32
	sz, _, _ := procGetFileVersionInfoSize.Call(uintptr(unsafe.Pointer(pUTF16)), uintptr(unsafe.Pointer(&dummy)))
	if sz == 0 || sz > 2<<20 {
		return "", ""
	}
	buf := make([]byte, sz)
	ok, _, _ := procGetFileVersionInfo.Call(uintptr(unsafe.Pointer(pUTF16)), 0, sz, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return "", ""
	}

	// Read translation table (\VarFileInfo\Translation) to pick the right lang-codepage block.
	langCP := "040904b0"
	if trPtr, trLen, ok := verQueryBytes(buf, `\VarFileInfo\Translation`); ok && trLen >= 4 {
		lang := *(*uint16)(trPtr)
		cp := *(*uint16)(unsafe.Add(trPtr, 2))
		langCP = hex4(lang) + hex4(cp)
	}

	tryPrefixes := []string{langCP, "040904b0", "040904e4", "080404b0", "000004b0"}
	for _, pref := range tryPrefixes {
		if company == "" {
			company = verQueryString(buf, `\StringFileInfo\`+pref+`\CompanyName`)
		}
		if fileVersion == "" {
			fileVersion = verQueryString(buf, `\StringFileInfo\`+pref+`\FileVersion`)
		}
		if company != "" && fileVersion != "" {
			break
		}
	}
	return strings.TrimSpace(company), strings.TrimSpace(fileVersion)
}

func verQueryBytes(buf []byte, subBlock string) (unsafe.Pointer, uint32, bool) {
	sb, err := syscall.UTF16PtrFromString(subBlock)
	if err != nil {
		return nil, 0, false
	}
	var ptr unsafe.Pointer
	var length uint32
	r, _, _ := procVerQueryValue.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(sb)),
		uintptr(unsafe.Pointer(&ptr)),
		uintptr(unsafe.Pointer(&length)),
	)
	if r == 0 || ptr == nil || length == 0 {
		return nil, 0, false
	}
	return ptr, length, true
}

func verQueryString(buf []byte, subBlock string) string {
	ptr, length, ok := verQueryBytes(buf, subBlock)
	if !ok {
		return ""
	}
	if length > 512 {
		length = 512
	}
	u16 := unsafe.Slice((*uint16)(ptr), length)
	return syscall.UTF16ToString(u16)
}

func hex4(v uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{
		digits[(v>>12)&0xF],
		digits[(v>>8)&0xF],
		digits[(v>>4)&0xF],
		digits[v&0xF],
	})
}
