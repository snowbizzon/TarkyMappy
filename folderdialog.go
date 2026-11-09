package main

import (
	"syscall"
	"unsafe"
)

var (
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

// comVtbl — первые 32 слота vtable (достаточно для IFileDialog/IShellItem).
type comVtbl = [32]uintptr

var (
	// CLSID_FileOpenDialog {DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}
	clsidFileOpenDialog = syscall.GUID{
		Data1: 0xDC1C5A9C,
		Data2: 0xE88A,
		Data3: 0x4DDE,
		Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7},
	}
	// IID_IFileOpenDialog {D57C7288-D4AD-4768-BE02-9D969532D960}
	iidFileOpenDialog = syscall.GUID{
		Data1: 0xD57C7288,
		Data2: 0xD4AD,
		Data3: 0x4768,
		Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60},
	}
)

// vtCall вызывает idx-ю функцию vtable (this = obj).
func vtCall(vt *comVtbl, obj unsafe.Pointer, idx int, args ...uintptr) {
	syscall.SyscallN(uintptr(vt[idx]), append([]uintptr{uintptr(obj)}, args...)...)
}

// vtCallRes вызывает idx-ю функцию и возвращает r1 (HRESULT-подобный код).
func vtCallRes(vt *comVtbl, obj unsafe.Pointer, idx int, args ...uintptr) uintptr {
	r1, _, _ := syscall.SyscallN(uintptr(vt[idx]), append([]uintptr{uintptr(obj)}, args...)...)
	return r1
}

// FolderDialog открывает нативный диалог выбора папки (COM IFileOpenDialog
// с FOS_PICKFOLDERS). ok=false — отмена или ошибка.
func FolderDialog() (path string, ok bool) {
	if hr, _, _ := procCoInitializeEx.Call(0, 2); hr != 0 && hr != 1 { // COINIT_APARTMENTTHREADED
		return "", false
	}
	defer procCoUninitialize.Call()

	var dlg unsafe.Pointer
	hr, _, _ := syscall.SyscallN(procCoCreateInstance.Addr(),
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if hr != 0 || dlg == nil {
		return "", false
	}
	vt := *(**comVtbl)(dlg)
	defer vtCall(vt, dlg, 2) // IUnknown::Release

	// IFileDialog: 10=GetOptions, 9=SetOptions, 3=Show, 20=GetResult.
	var opts uintptr
	vtCallRes(vt, dlg, 10, uintptr(unsafe.Pointer(&opts)))
	vtCall(vt, dlg, 9, opts|0x20) // FOS_PICKFOLDERS
	if vtCallRes(vt, dlg, 3, 0) != 0 {
		return "", false // отменено или ошибка
	}
	var item unsafe.Pointer
	if vtCallRes(vt, dlg, 20, uintptr(unsafe.Pointer(&item))) != 0 || item == nil {
		return "", false
	}
	vt2 := *(**comVtbl)(item)
	defer vtCall(vt2, item, 2) // Release

	// IShellItem: 5=GetDisplayName(SIGDN_FILESYSPATH=0x80058000).
	var pPath *uint16
	if vtCallRes(vt2, item, 5, 0x80058000, uintptr(unsafe.Pointer(&pPath))) != 0 || pPath == nil {
		return "", false
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(pPath)))
	return utf16PtrToString(pPath), true
}

// utf16PtrToString — NUL-terminated *uint16 -> string.
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; n++ {
		ptr = unsafe.Pointer(uintptr(ptr) + 2)
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}
