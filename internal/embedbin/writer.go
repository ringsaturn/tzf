package embedbin

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

type OutSection struct {
	Typ   uint32
	Data  []byte
	Align uint64
}

// AssembleFile lays the sections out behind the shared 64-byte header and
// section table, and seals the file with its CRC32 footer.
func AssembleFile(profile byte, flags, chunkTarget uint32, tzCount int, version string, sections []OutSection) ([]byte, error) {
	offsets := make([]uint32, len(sections))
	cursor := uint64(headerSize) + uint64(len(sections))*sectionEntryLen
	for i, section := range sections {
		cursor = alignUp(cursor, section.Align)
		off, err := CheckedU32("section offset", cursor)
		if err != nil {
			return nil, err
		}
		offsets[i] = off
		cursor += uint64(len(section.Data))
	}
	fileSize := cursor + footerSize
	size32, err := CheckedU32("file size", fileSize)
	if err != nil {
		return nil, err
	}
	if fileSize > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("file size: %w: host int capacity", ErrMalformed)
	}
	out := make([]byte, int(fileSize))
	copy(out[0:4], "TZFB")
	out[4], out[5] = formatMajor, formatMinor
	binary.LittleEndian.PutUint16(out[6:], headerSize)
	binary.LittleEndian.PutUint32(out[8:], flags)
	binary.LittleEndian.PutUint32(out[12:], coordScale)
	binary.LittleEndian.PutUint32(out[16:], size32)
	binary.LittleEndian.PutUint32(out[20:], uint32(len(sections)))
	copy(out[24:40], version)
	binary.LittleEndian.PutUint32(out[40:], uint32(tzCount))
	binary.LittleEndian.PutUint32(out[44:], chunkTarget)
	out[profileOffset] = profile
	for i, section := range sections {
		o := headerSize + i*sectionEntryLen
		binary.LittleEndian.PutUint32(out[o:], section.Typ)
		binary.LittleEndian.PutUint32(out[o+4:], offsets[i])
		binary.LittleEndian.PutUint32(out[o+8:], uint32(len(section.Data)))
		copy(out[offsets[i]:], section.Data)
	}
	binary.LittleEndian.PutUint32(out[len(out)-4:], crc32.ChecksumIEEE(out[:len(out)-4]))
	return out, nil
}
