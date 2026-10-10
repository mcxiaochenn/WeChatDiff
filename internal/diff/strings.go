package diff

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

func u32(b []byte, off int) (uint32, error) {
	if off < 0 || off > len(b)-4 {
		return 0, fmt.Errorf("二进制偏移越界")
	}
	return binary.LittleEndian.Uint32(b[off:]), nil
}
func dexStrings(b []byte) ([]string, error) {
	if len(b) < 112 || string(b[:4]) != "dex\x0a" {
		return nil, fmt.Errorf("不支持的 DEX 格式")
	}
	size, e := u32(b, 56)
	if e != nil {
		return nil, e
	}
	off, e := u32(b, 60)
	if e != nil {
		return nil, e
	}
	if size > 2000000 || uint64(off)+uint64(size)*4 > uint64(len(b)) {
		return nil, fmt.Errorf("DEX 字符串表越界")
	}
	values := make([]string, 0, size)
	for i := uint32(0); i < size; i++ {
		p, _ := u32(b, int(off+i*4))
		pos := int(p)
		// string_data_item: ULEB128 UTF-16 长度，随后 MUTF-8。
		done := false
		for k := 0; k < 5 && pos < len(b); k++ {
			c := b[pos]
			pos++
			if c < 128 {
				done = true
				break
			}
		}
		if !done {
			return nil, fmt.Errorf("损坏的 DEX 字符串长度")
		}
		start := pos
		for pos < len(b) && b[pos] != 0 {
			pos++
		}
		if pos == len(b) {
			return nil, fmt.Errorf("DEX 字符串未终止")
		}
		values = append(values, decodeMUTF8(b[start:pos]))
	}
	return values, nil
}
func decodeMUTF8(b []byte) string {
	var units []uint16
	for i := 0; i < len(b); {
		c := b[i]
		i++
		switch {
		case c < 128:
			units = append(units, uint16(c))
		case c&0xe0 == 0xc0 && i < len(b):
			units = append(units, uint16(c&31)<<6|uint16(b[i]&63))
			i++
		case c&0xf0 == 0xe0 && i+1 < len(b):
			units = append(units, uint16(c&15)<<12|uint16(b[i]&63)<<6|uint16(b[i+1]&63))
			i += 2
		default:
			units = append(units, 0xfffd)
		}
	}
	return string(utf16.Decode(units))
}
func resourceStrings(b []byte) ([]string, error) {
	if len(b) < 12 {
		return nil, fmt.Errorf("资源表过短")
	}
	header := int(binary.LittleEndian.Uint16(b[2:4]))
	if header < 8 || header > len(b)-28 {
		return nil, fmt.Errorf("资源表头越界")
	}
	p := b[header:]
	if binary.LittleEndian.Uint16(p) != 1 {
		return nil, fmt.Errorf("资源表缺少全局字符串池")
	}
	return stringPool(p)
}
func stringPool(p []byte) ([]string, error) {
	if len(p) < 28 {
		return nil, fmt.Errorf("字符串池过短")
	}
	size, _ := u32(p, 4)
	count, _ := u32(p, 8)
	flags, _ := u32(p, 16)
	start, _ := u32(p, 20)
	hdr := int(binary.LittleEndian.Uint16(p[2:4]))
	if uint64(size) > uint64(len(p)) || size < 28 || count > 2000000 || hdr < 28 || uint64(hdr)+uint64(count)*4 > uint64(size) {
		return nil, fmt.Errorf("字符串池越界")
	}
	p = p[:size]
	var out []string
	for i := uint32(0); i < count; i++ {
		off, _ := u32(p, hdr+int(i)*4)
		pos := int(uint64(start) + uint64(off))
		if pos >= len(p) {
			return nil, fmt.Errorf("资源字符串越界")
		}
		readLen := func(wide bool) (int, error) {
			if wide {
				if pos+2 > len(p) {
					return 0, fmt.Errorf("UTF16 长度越界")
				}
				v := int(binary.LittleEndian.Uint16(p[pos:]))
				pos += 2
				if v&0x8000 != 0 {
					if pos+2 > len(p) {
						return 0, fmt.Errorf("UTF16 长度越界")
					}
					v = (v&0x7fff)<<16 | int(binary.LittleEndian.Uint16(p[pos:]))
					pos += 2
				}
				return v, nil
			}
			if pos >= len(p) {
				return 0, fmt.Errorf("UTF8 长度越界")
			}
			v := int(p[pos])
			pos++
			if v&128 != 0 {
				if pos >= len(p) {
					return 0, fmt.Errorf("UTF8 长度越界")
				}
				v = (v&127)<<8 | int(p[pos])
				pos++
			}
			return v, nil
		}
		if flags&256 != 0 {
			if _, e := readLen(false); e != nil {
				return nil, e
			}
			n, e := readLen(false)
			if e != nil {
				return nil, e
			}
			if n > len(p)-pos {
				return nil, fmt.Errorf("UTF8 内容越界")
			}
			out = append(out, string(p[pos:pos+n]))
		} else {
			n, e := readLen(true)
			if e != nil {
				return nil, e
			}
			if n > (len(p)-pos)/2 {
				return nil, fmt.Errorf("UTF16 内容越界")
			}
			u := make([]uint16, n)
			for j := range u {
				u[j] = binary.LittleEndian.Uint16(p[pos+j*2:])
			}
			out = append(out, string(utf16.Decode(u)))
		}
	}
	return out, nil
}
