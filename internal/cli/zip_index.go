package cli

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
)

// archive/zip создаёт объект для каждой записи каталога в NewReader.
// До этого вызова проверяем счётчик и обходим заголовки фиксированным буфером.
// Содержимое файлов, сжатие, CRC и поля самих записей разбирает archive/zip.
func checkZipIndex(r io.ReaderAt, size int64) error {
	const endSize = 22
	tail := make([]byte, min(size, 65*1024))
	if len(tail) < endSize {
		return zip.ErrFormat
	}
	tailStart := size - int64(len(tail))
	if _, err := r.ReadAt(tail, tailStart); err != nil {
		return err
	}
	end := bytes.LastIndex(tail[:len(tail)-endSize+4], []byte("PK\x05\x06"))
	if end < 0 {
		return zip.ErrFormat
	}
	footer := tail[end:]
	le := binary.LittleEndian
	if endSize+int(le.Uint16(footer[20:])) > len(footer) {
		return zip.ErrFormat
	}
	endPos := tailStart + int64(end)
	count := uint64(le.Uint16(footer[10:]))
	length, offset := uint64(le.Uint32(footer[12:])), uint64(le.Uint32(footer[16:]))
	if count == 0xffff || length == 0xffff || length == 0xffffffff || offset == 0xffffffff {
		// В ZIP64 смещение расширенного заголовка находится в локаторе перед обычным окончанием.
		var loc [20]byte
		if endPos >= 20 {
			if _, err := r.ReadAt(loc[:], endPos-20); err != nil {
				return err
			}
			if string(loc[:4]) == "PK\x06\x07" {
				pos := le.Uint64(loc[8:])
				if pos > uint64(endPos) {
					return zip.ErrFormat
				}
				var info [56]byte
				if _, err := r.ReadAt(info[:], int64(pos)); err != nil {
					return err
				}
				if string(info[:4]) != "PK\x06\x06" || le.Uint32(loc[4:]) != 0 || le.Uint32(loc[16:]) != 1 {
					return zip.ErrFormat
				}
				endPos = int64(pos)
				count = le.Uint64(info[32:])
				length = le.Uint64(info[40:])
				offset = le.Uint64(info[48:])
			}
		}
	}
	if count > maxEntries || length > maxBytes {
		return containerLimit()
	}
	if length > uint64(endPos) || offset > uint64(size) {
		return zip.ErrFormat
	}
	start := endPos - int64(length)
	if err := countZipHeaders(r, size, start); err != nil {
		return err
	}
	// Для архивов с префиксом Go также проверяет каталог по исходному смещению.
	if int64(offset) < start {
		return countZipHeaders(r, size, int64(offset))
	}
	return nil
}

func countZipHeaders(r io.ReaderAt, size, start int64) error {
	const headerSize = 46
	var h [headerSize]byte
	for pos, n := start, 0; pos <= size-headerSize; n++ {
		if _, err := r.ReadAt(h[:], pos); err != nil {
			return err
		}
		if string(h[:4]) != "PK\x01\x02" {
			break
		}
		if n >= maxEntries {
			return containerLimit()
		}
		le := binary.LittleEndian
		extent := int64(headerSize) + int64(le.Uint16(h[28:])) + int64(le.Uint16(h[30:])) + int64(le.Uint16(h[32:]))
		if extent > size-pos {
			return zip.ErrFormat
		}
		pos += extent
		if pos-start > maxBytes {
			return containerLimit()
		}
	}
	return nil
}
