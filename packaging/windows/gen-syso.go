//go:build ignore

// gen-syso.go converts assets/brand/memora.ico into cmd/memora/rsrc_windows_amd64.syso
// so Go's PE linker embeds the MEMORA brand icon (RT_ICON/RT_GROUP_ICON),
// authoritative VS_VERSION_INFO metadata (RT_VERSION), and Windows 10/11
// PerMonitorV2 application manifest (RT_MANIFEST).
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf16"
)

type iconDirEntry struct {
	Width      uint8
	Height     uint8
	ColorCount uint8
	Reserved   uint8
	Planes     uint16
	BitCount   uint16
	BytesInRes uint32
	ImageOff   uint32
}

const appManifestXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="0.3.0.0" processorArchitecture="amd64" name="MEMORA.Desktop.Agent" type="win32"/>
  <description>MEMORA 智能性能调度与物理内存真相中心</description>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
      <supportedOS Id="{1f676c76-80e1-4239-95bb-83d0f6d0da78}"/>
      <supportedOS Id="{4a2f28e3-53b9-4441-ba9c-d69d4a4a6e38}"/>
      <supportedOS Id="{35138b9a-5d96-4fbd-8e2d-a2440225f93a}"/>
    </application>
  </compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2, PerMonitor</dpiAwareness>
    </windowsSettings>
  </application>
</assembly>
`

func main() {
	icoPath := filepath.Join("assets", "brand", "memora.ico")
	outPath := filepath.Join("cmd", "memora", "rsrc_windows_amd64.syso")

	raw, err := os.ReadFile(icoPath)
	if err != nil {
		fatalf("read %s: %v", icoPath, err)
	}
	if len(raw) < 6 {
		fatalf("invalid ico: too short")
	}
	rType := binary.LittleEndian.Uint16(raw[2:4])
	count := int(binary.LittleEndian.Uint16(raw[4:6]))
	if rType != 1 || count <= 0 || len(raw) < 6+count*16 {
		fatalf("invalid ico header (type=%d count=%d)", rType, count)
	}

	entries := make([]iconDirEntry, count)
	images := make([][]byte, count)
	for i := 0; i < count; i++ {
		off := 6 + i*16
		e := iconDirEntry{
			Width:      raw[off+0],
			Height:     raw[off+1],
			ColorCount: raw[off+2],
			Reserved:   raw[off+3],
			Planes:     binary.LittleEndian.Uint16(raw[off+4 : off+6]),
			BitCount:   binary.LittleEndian.Uint16(raw[off+6 : off+8]),
			BytesInRes: binary.LittleEndian.Uint32(raw[off+8 : off+12]),
			ImageOff:   binary.LittleEndian.Uint32(raw[off+12 : off+16]),
		}
		end := int(e.ImageOff) + int(e.BytesInRes)
		if int(e.ImageOff) < 0 || end > len(raw) {
			fatalf("ico entry %d out of range", i)
		}
		entries[i] = e
		images[i] = raw[e.ImageOff:end]
	}

	// Build RT_GROUP_ICON payload: 6-byte header + count * 14-byte GRPICONDIRENTRY.
	var grp bytes.Buffer
	_ = binary.Write(&grp, binary.LittleEndian, uint16(0))
	_ = binary.Write(&grp, binary.LittleEndian, uint16(1))
	_ = binary.Write(&grp, binary.LittleEndian, uint16(count))
	for i, e := range entries {
		grp.WriteByte(e.Width)
		grp.WriteByte(e.Height)
		grp.WriteByte(e.ColorCount)
		grp.WriteByte(e.Reserved)
		_ = binary.Write(&grp, binary.LittleEndian, e.Planes)
		_ = binary.Write(&grp, binary.LittleEndian, e.BitCount)
		_ = binary.Write(&grp, binary.LittleEndian, e.BytesInRes)
		_ = binary.Write(&grp, binary.LittleEndian, uint16(i+1))
	}
	groupBytes := grp.Bytes()
	versionBytes := buildVersionInfoBlob()
	manifestBytes := []byte(appManifestXML)

	// Layout inside .rsrc section:
	//   rootDir:    16 + 4*8 = 48 bytes (sorted IDs: 3=RT_ICON, 14=RT_GROUP_ICON, 16=RT_VERSION, 24=RT_MANIFEST)
	//   typeDir3:   16 + count*8 bytes (IDs 1..count)
	//   typeDir14:  16 + 1*8 = 24 bytes (ID 1)
	//   typeDir16:  16 + 1*8 = 24 bytes (ID 1)
	//   typeDir24:  16 + 1*8 = 24 bytes (ID 1)
	//   langDirs:   totalItems * 24 bytes (Lang 0x0409)
	//   dataEnts:   totalItems * 16 bytes
	//   payloads:   each 4-byte aligned
	totalItems := count + 3
	rootOff := uint32(0)
	rootSize := uint32(16 + 4*8)
	type3Off := rootOff + rootSize
	type3Size := uint32(16 + count*8)
	type14Off := type3Off + type3Size
	type14Size := uint32(16 + 1*8)
	type16Off := type14Off + type14Size
	type16Size := uint32(16 + 1*8)
	type24Off := type16Off + type16Size
	type24Size := uint32(16 + 1*8)
	langOff := type24Off + type24Size
	langUnit := uint32(16 + 8)
	dataEntOff := langOff + uint32(totalItems)*langUnit
	payloadOff := dataEntOff + uint32(totalItems)*16

	payloads := make([][]byte, totalItems)
	for i := 0; i < count; i++ {
		payloads[i] = images[i]
	}
	payloads[count] = groupBytes
	payloads[count+1] = versionBytes
	payloads[count+2] = manifestBytes

	payloadOffsets := make([]uint32, totalItems)
	payloadSizes := make([]uint32, totalItems)
	cur := payloadOff
	for i, p := range payloads {
		payloadOffsets[i] = cur
		payloadSizes[i] = uint32(len(p))
		cur = align4(cur + uint32(len(p)))
	}
	totalRsrc := cur

	rsrc := make([]byte, totalRsrc)
	writeDirHeader(rsrc[rootOff:], 0, 4)
	writeDirEntry(rsrc[rootOff+16:], 3, 0x80000000|type3Off)
	writeDirEntry(rsrc[rootOff+24:], 14, 0x80000000|type14Off)
	writeDirEntry(rsrc[rootOff+32:], 16, 0x80000000|type16Off)
	writeDirEntry(rsrc[rootOff+40:], 24, 0x80000000|type24Off)

	writeDirHeader(rsrc[type3Off:], 0, uint16(count))
	for i := 0; i < count; i++ {
		lOff := langOff + uint32(i)*langUnit
		writeDirEntry(rsrc[type3Off+16+uint32(i*8):], uint32(i+1), 0x80000000|lOff)
	}

	writeDirHeader(rsrc[type14Off:], 0, 1)
	writeDirEntry(rsrc[type14Off+16:], 1, 0x80000000|(langOff+uint32(count)*langUnit))

	writeDirHeader(rsrc[type16Off:], 0, 1)
	writeDirEntry(rsrc[type16Off+16:], 1, 0x80000000|(langOff+uint32(count+1)*langUnit))

	writeDirHeader(rsrc[type24Off:], 0, 1)
	writeDirEntry(rsrc[type24Off+16:], 1, 0x80000000|(langOff+uint32(count+2)*langUnit))

	var relocs []uint32
	for i := 0; i < totalItems; i++ {
		lOff := langOff + uint32(i)*langUnit
		dOff := dataEntOff + uint32(i*16)
		writeDirHeader(rsrc[lOff:], 0, 1)
		writeDirEntry(rsrc[lOff+16:], 0x0409, dOff)

		// IMAGE_RESOURCE_DATA_ENTRY (RVA relocated via IMAGE_REL_AMD64_ADDR32NB)
		binary.LittleEndian.PutUint32(rsrc[dOff:], payloadOffsets[i])
		binary.LittleEndian.PutUint32(rsrc[dOff+4:], payloadSizes[i])
		relocs = append(relocs, dOff)

		copy(rsrc[payloadOffsets[i]:], payloads[i])
	}

	// Build COFF amd64 object file: 20B FileHeader + 40B SectionHeader + rsrc + relocs + 18B sym + 4B strtab.
	var obj bytes.Buffer
	ptrRaw := uint32(20 + 40)
	ptrReloc := ptrRaw + totalRsrc
	ptrSyms := ptrReloc + uint32(len(relocs)*10)

	// IMAGE_FILE_HEADER
	_ = binary.Write(&obj, binary.LittleEndian, uint16(0x8664)) // Machine = AMD64
	_ = binary.Write(&obj, binary.LittleEndian, uint16(1))      // NumberOfSections
	_ = binary.Write(&obj, binary.LittleEndian, uint32(0))      // TimeDateStamp
	_ = binary.Write(&obj, binary.LittleEndian, ptrSyms)        // PointerToSymbolTable
	_ = binary.Write(&obj, binary.LittleEndian, uint32(1))      // NumberOfSymbols
	_ = binary.Write(&obj, binary.LittleEndian, uint16(0))      // SizeOfOptionalHeader
	_ = binary.Write(&obj, binary.LittleEndian, uint16(0))      // Characteristics

	// IMAGE_SECTION_HEADER (.rsrc)
	var secName [8]byte
	copy(secName[:], ".rsrc")
	obj.Write(secName[:])
	_ = binary.Write(&obj, binary.LittleEndian, uint32(0))          // VirtualSize
	_ = binary.Write(&obj, binary.LittleEndian, uint32(0))          // VirtualAddress
	_ = binary.Write(&obj, binary.LittleEndian, totalRsrc)          // SizeOfRawData
	_ = binary.Write(&obj, binary.LittleEndian, ptrRaw)             // PointerToRawData
	_ = binary.Write(&obj, binary.LittleEndian, ptrReloc)           // PointerToRelocations
	_ = binary.Write(&obj, binary.LittleEndian, uint32(0))          // PointerToLinenumbers
	_ = binary.Write(&obj, binary.LittleEndian, uint16(len(relocs))) // NumberOfRelocations
	_ = binary.Write(&obj, binary.LittleEndian, uint16(0))          // NumberOfLinenumbers
	_ = binary.Write(&obj, binary.LittleEndian, uint32(0x40000040)) // CNT_INITIALIZED_DATA | MEM_READ

	obj.Write(rsrc)

	for _, rva := range relocs {
		_ = binary.Write(&obj, binary.LittleEndian, rva)       // VirtualAddress
		_ = binary.Write(&obj, binary.LittleEndian, uint32(0)) // SymbolTableIndex = 0 (.rsrc)
		_ = binary.Write(&obj, binary.LittleEndian, uint16(3)) // IMAGE_REL_AMD64_ADDR32NB
	}

	// Single STATIC symbol ".rsrc"
	obj.Write(secName[:])
	_ = binary.Write(&obj, binary.LittleEndian, uint32(0))
	_ = binary.Write(&obj, binary.LittleEndian, uint16(1)) // SectionNumber = 1
	_ = binary.Write(&obj, binary.LittleEndian, uint16(0)) // Type
	obj.WriteByte(3)                                        // IMAGE_SYM_CLASS_STATIC
	obj.WriteByte(0)                                        // NumberOfAuxSymbols
	_ = binary.Write(&obj, binary.LittleEndian, uint32(4)) // Empty string table size = 4

	if err := os.WriteFile(outPath, obj.Bytes(), 0o644); err != nil {
		fatalf("write %s: %v", outPath, err)
	}
	fmt.Printf("generated %s (%d bytes, %d icons + RT_VERSION + RT_MANIFEST)\n", outPath, obj.Len(), count)
}

func buildVersionInfoBlob() []byte {
	stringsList := [][2]string{
		{"CompanyName", "MEMORA Project"},
		{"FileDescription", "MEMORA 智能性能调度与物理内存真相中心"},
		{"FileVersion", "0.3.0.0"},
		{"InternalName", "memora"},
		{"LegalCopyright", "Copyright (c) 2026 MEMORA Project (MIT License)"},
		{"OriginalFilename", "memora.exe"},
		{"ProductName", "MEMORA"},
		{"ProductVersion", "0.3.0"},
	}

	var stringNodes [][]byte
	for _, kv := range stringsList {
		stringNodes = append(stringNodes, buildVersionStringNode(kv[0], kv[1]))
	}
	stringTable := buildVersionBlock("040904B0", 0, 1, nil, stringNodes)
	stringFileInfo := buildVersionBlock("StringFileInfo", 0, 1, nil, [][]byte{stringTable})

	var varVal bytes.Buffer
	_ = binary.Write(&varVal, binary.LittleEndian, uint16(0x0409))
	_ = binary.Write(&varVal, binary.LittleEndian, uint16(0x04B0))
	varNode := buildVersionBlock("Translation", uint16(varVal.Len()), 0, varVal.Bytes(), nil)
	varFileInfo := buildVersionBlock("VarFileInfo", 0, 1, nil, [][]byte{varNode})

	var ffi bytes.Buffer
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0xFEEF04BD)) // dwSignature
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x00010000)) // dwStrucVersion
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x00000003)) // dwFileVersionMS (0.3)
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x00000000)) // dwFileVersionLS (0.0)
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x00000003)) // dwProductVersionMS (0.3)
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x00000000)) // dwProductVersionLS (0.0)
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x0000003F)) // dwFileFlagsMask
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0))          // dwFileFlags
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x00040004)) // dwFileOS = VOS_NT_WINDOWS32
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0x00000001)) // dwFileType = VFT_APP
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0))          // dwFileSubtype
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0))          // dwFileDateMS
	_ = binary.Write(&ffi, binary.LittleEndian, uint32(0))          // dwFileDateLS

	return buildVersionBlock("VS_VERSION_INFO", uint16(ffi.Len()), 0, ffi.Bytes(), [][]byte{stringFileInfo, varFileInfo})
}

func buildVersionStringNode(key, value string) []byte {
	u16Val := utf16.Encode([]rune(value + "\x00"))
	var valBuf bytes.Buffer
	for _, ch := range u16Val {
		_ = binary.Write(&valBuf, binary.LittleEndian, ch)
	}
	return buildVersionBlock(key, uint16(len(u16Val)), 1, valBuf.Bytes(), nil)
}

func buildVersionBlock(key string, wValueLength, wType uint16, value []byte, children [][]byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint16(0)) // placeholder for wLength
	_ = binary.Write(&b, binary.LittleEndian, wValueLength)
	_ = binary.Write(&b, binary.LittleEndian, wType)
	for _, ch := range utf16.Encode([]rune(key + "\x00")) {
		_ = binary.Write(&b, binary.LittleEndian, ch)
	}
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
	if len(value) > 0 {
		b.Write(value)
	}
	for i, child := range children {
		if i == 0 || b.Len()%4 != 0 {
			for b.Len()%4 != 0 {
				b.WriteByte(0)
			}
		}
		b.Write(child)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out[0:2], uint16(len(out)))
	return out
}

func align4(n uint32) uint32 {
	return (n + 3) &^ 3
}

func writeDirHeader(b []byte, named, id uint16) {
	binary.LittleEndian.PutUint16(b[12:], named)
	binary.LittleEndian.PutUint16(b[14:], id)
}

func writeDirEntry(b []byte, id, offset uint32) {
	binary.LittleEndian.PutUint32(b[0:], id)
	binary.LittleEndian.PutUint32(b[4:], offset)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
