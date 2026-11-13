package winmem

import (
	"syscall"
	"unsafe"
)

var (
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
	procFreeConsole  = kernel32.NewProc("FreeConsole")
	procCloseHandle  = kernel32.NewProc("CloseHandle")
)

const errorAlreadyExists = 183

// HideConsole detaches the process from its console window. Used when MEMORA
// starts from the Run key, where a visible console would be noise.
func HideConsole() {
	procFreeConsole.Call()
}

// AcquireSingleInstance takes a named mutex. The returned release function
// must be called on exit; ok is false when another instance already owns it.
func AcquireSingleInstance(name string) (release func(), ok bool) {
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return func() {}, false
	}
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(ptr)))
	if h == 0 {
		return func() {}, false
	}
	if errno, isErrno := callErr.(syscall.Errno); isErrno && uintptr(errno) == errorAlreadyExists {
		procCloseHandle.Call(h)
		return func() {}, false
	}
	return func() { procCloseHandle.Call(h) }, true
}
