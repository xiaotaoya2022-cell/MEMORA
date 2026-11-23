package proc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	procSetProcessWorkingSetSizeEx = kernel32.NewProc("SetProcessWorkingSetSizeEx")

	gitPathOnce      sync.Once
	gitPathOptimized bool
	gitPathNote      string
)

// ApplySoftWorkingSetCap sets a soft working-set quota ceiling (Flags=0, without
// QUOTA_LIMITS_HARDWS_MAX_ENABLE) on an idle or rebound-prone background process.
// When physical RAM is plentiful Windows allows the process to fault pages normally
// without disk thrashing, while MmWorkingSetManager automatically trims it back to
// maxBytes under background/idle conditions.
func ApplySoftWorkingSetCap(pid uint32, minBytes, maxBytes uint64) error {
	if pid <= 4 {
		return fmt.Errorf("refusing to cap system pid %d", pid)
	}
	if err := procSetProcessWorkingSetSizeEx.Find(); err != nil {
		return err
	}
	if minBytes < 4<<20 {
		minBytes = 4 << 20
	}
	if maxBytes < minBytes+(8<<20) {
		maxBytes = minBytes + (8 << 20)
	}
	h, _, err := procOpenProcess.Call(processSetQuota|processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return err
	}
	defer procCloseHandle.Call(h)

	// Flags = 0 (soft min/max quota, never hard-locks or starves pages).
	r, _, callErr := procSetProcessWorkingSetSizeEx.Call(h, uintptr(minBytes), uintptr(maxBytes), 0)
	if r == 0 {
		return callErr
	}
	return nil
}

// ClearSoftWorkingSetCap restores a process's default automatic OS working-set quota
// by passing SIZE_T(-1), SIZE_T(-1) to SetProcessWorkingSetSizeEx.
func ClearSoftWorkingSetCap(pid uint32) error {
	if pid <= 4 {
		return nil
	}
	if err := procSetProcessWorkingSetSizeEx.Find(); err != nil {
		return err
	}
	h, _, err := procOpenProcess.Call(processSetQuota|processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return err
	}
	defer procCloseHandle.Call(h)

	allOnes := ^uintptr(0)
	r, _, callErr := procSetProcessWorkingSetSizeEx.Call(h, allOnes, allOnes, 0)
	if r == 0 {
		return callErr
	}
	return nil
}

// EnsureGitDirectPath checks whether Git for Windows is installed with its 46 KB
// bin\git.exe / cmd\git.exe shim launcher ahead of mingw64\bin\git.exe in PATH.
// Placing mingw64\bin directly ahead of Git\bin in PATH eliminates 50% of git.exe
// process creations (1 process instead of 2 per git status poll) with zero functional change.
func EnsureGitDirectPath() (bool, string) {
	gitPathOnce.Do(func() {
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		mingwBin := filepath.Join(pf, "Git", "mingw64", "bin")
		mingwGit := filepath.Join(mingwBin, "git.exe")
		shimGit := filepath.Join(pf, "Git", "bin", "git.exe")

		if info, err := os.Stat(mingwGit); err != nil || info.IsDir() {
			return
		}
		if info, err := os.Stat(shimGit); err != nil || info.IsDir() {
			return
		}

		curPath := os.Getenv("PATH")
		parts := strings.Split(curPath, ";")
		mingwIdx := -1
		shimIdx := -1
		for i, p := range parts {
			clean := strings.TrimRight(strings.TrimSpace(p), `\`)
			if strings.EqualFold(clean, mingwBin) && mingwIdx < 0 {
				mingwIdx = i
			}
			if (strings.EqualFold(clean, filepath.Join(pf, "Git", "bin")) ||
				strings.EqualFold(clean, filepath.Join(pf, "Git", "cmd"))) && shimIdx < 0 {
				shimIdx = i
			}
		}
		if shimIdx >= 0 && (mingwIdx < 0 || mingwIdx > shimIdx) {
			next := make([]string, 0, len(parts)+1)
			for i, p := range parts {
				if i == shimIdx {
					next = append(next, mingwBin)
				}
				if mingwIdx >= 0 && i == mingwIdx {
					continue
				}
				next = append(next, p)
			}
			_ = os.Setenv("PATH", strings.Join(next, ";"))
		}
		gitPathOptimized = true
		gitPathNote = mingwGit
	})
	return gitPathOptimized, gitPathNote
}
