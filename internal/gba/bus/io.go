package bus

import "encoding/binary"

type read16Handler func() uint16
type write16Handler func(uint16)
type write8Handler func(uint32, byte)

// IO is the 1KB GBA I/O register block. Unknown registers retain ordinary
// byte-backed storage until a hardware component installs a handler.
type IO struct {
	data    [IOSize]byte
	read16  map[uint32]read16Handler
	write16 map[uint32]write16Handler
	write8   map[uint32]write8Handler
}

func NewIO() *IO {
	return &IO{
		read16:  make(map[uint32]read16Handler),
		write16: make(map[uint32]write16Handler),
		write8:   make(map[uint32]write8Handler),
	}
}

// Register16 installs optional read/write callbacks for one aligned halfword.
func (io *IO) Register16(offset uint32, read func() uint16, write func(uint16)) {
	io.Register16WithByteWrite(offset, read, write, nil)
}

// Register16WithByteWrite installs a 16-bit register and optionally overrides
// byte writes. Most registers can use the default read-modify-write behavior;
// write-one-to-clear registers such as IF need to know which byte was written.
func (io *IO) Register16WithByteWrite(offset uint32, read func() uint16, write func(uint16), write8 func(byteOffset uint32, value byte)) {
	if offset >= IOSize || offset&1 != 0 {
		panic("gba bus: invalid 16-bit I/O register offset")
	}
	if read != nil {
		io.read16[offset] = read
	}
	if write != nil {
		io.write16[offset] = write
	}
	if write8 != nil {
		io.write8[offset] = write8
	}
}

func (io *IO) Read8(offset uint32) (byte, bool) {
	if offset >= IOSize {
		return 0, false
	}
	base := offset &^ 1
	if read := io.read16[base]; read != nil {
		value := read()
		if offset&1 != 0 {
			return byte(value >> 8), true
		}
		return byte(value), true
	}
	return io.data[offset], true
}

func (io *IO) Write8(offset uint32, value byte) {
	if offset >= IOSize {
		return
	}
	base := offset &^ 1
	if write := io.write8[base]; write != nil {
		write(offset-base, value)
		return
	}
	if write := io.write16[base]; write != nil {
		current, _ := io.Read16(base)
		if offset&1 == 0 {
			current = current&0xff00 | uint16(value)
		} else {
			current = current&0x00ff | uint16(value)<<8
		}
		write(current)
		return
	}
	io.data[offset] = value
}

func (io *IO) Read16(offset uint32) (uint16, bool) {
	if offset >= IOSize-1 {
		return 0, false
	}
	offset &^= 1
	if read := io.read16[offset]; read != nil {
		return read(), true
	}
	return binary.LittleEndian.Uint16(io.data[offset : offset+2]), true
}

func (io *IO) Write16(offset uint32, value uint16) {
	if offset >= IOSize-1 {
		return
	}
	offset &^= 1
	if write := io.write16[offset]; write != nil {
		write(value)
		return
	}
	binary.LittleEndian.PutUint16(io.data[offset:offset+2], value)
}

func (io *IO) Read32(offset uint32) (uint32, bool) {
	if offset >= IOSize-3 {
		return 0, false
	}
	offset &^= 3
	lo, ok0 := io.Read16(offset)
	hi, ok1 := io.Read16(offset + 2)
	return uint32(lo) | uint32(hi)<<16, ok0 && ok1
}

func (io *IO) Write32(offset uint32, value uint32) {
	if offset >= IOSize-3 {
		return
	}
	offset &^= 3
	io.Write16(offset, uint16(value))
	io.Write16(offset+2, uint16(value>>16))
}
