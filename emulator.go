package x86_emulator

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
)

// ref1. https://en.wikibooks.org/wiki/X86_Assembly/Machine_Language_Conversion
// ref2. https://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-instruction-set-reference-manual-325383.pdf

const (
	paragraphSize int = 16
)

// for debug log
type debugT bool
var debug = debugT(false)
func (d debugT) printf(format string, args ...interface{}) {
	if d {
		log.Printf(format, args...)
	}
}

type word uint16
type dword uint32
type exitCode uint8

type segmentOverride struct {
	sreg registerS
}

type exe struct {
	rawHeader []byte
	header header
}

func realAddress(seg word, offset word) address {
	return address(seg) << 4 + address(offset)
}

// --- parser

type parser struct {
	sc *bufio.Scanner
	offset int
}

func newParser(reader io.Reader) *parser {
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)
	return &parser{
		sc: sc,
		offset: 0,
	}
}

func (parser *parser) parseBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		if b := parser.sc.Scan(); b {
			buf[i] = parser.sc.Bytes()[0]
			parser.offset++
		} else {
			if err := parser.sc.Err(); err != nil {
				return nil, errors.Wrap(err, "failed to parse bytes")
			} else {
				return nil, io.EOF
			}
		}
	}
	return buf, nil
}

func (parser *parser) parseByte() (byte, error) {
	bs, err := parser.parseBytes(1)
	if err != nil {
		if err := parser.sc.Err(); err != nil {
			return 0, errors.Wrap(err, "failed to parse byte")
		} else {
			return 0, io.EOF
		}
	}
	return bs[0], nil
}

// assume little-endian
func (parser *parser) parseWord() (word, error) {
	buf, err := parser.parseBytes(2)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse word")
	}
	return word(buf[1]) << 8 + word(buf[0]), nil
}

func (parser *parser) parseRemains() ([]byte, error) {
	var buf []byte
	for {
		byte, err := parser.parseByte()
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			} else {
				return nil, errors.Wrap(err, "failed to parse")
			}
		}
		buf = append(buf, byte)
	}
	return buf, nil
}

// --- header

type header struct {
	exSignature [2]byte
	relocationItems word
	exHeaderSize word
	exInitSS word
	exInitSP word
	exInitIP word
	exInitCS word
	relocationTableOffset word
}

func (h header) String() string {
	return fmt.Sprintf("header{exSignature: %v, exHeaderSize: %d, exInitSS: 0x%04X, exInitSP: 0x%04X, exInitIP: 0x%04X, exInitCS: 0x%04X}",
		h.exSignature, h.exHeaderSize, h.exInitSS, h.exInitSP, h.exInitIP, h.exInitCS)
}

// header, load module, error
func parseHeader(reader io.Reader) (*header, []byte, error) {
	parser := newParser(reader)
	return parseHeaderWithParser(parser)
}

// header, load module, error
func parseHeaderWithParser(parser *parser) (*header, []byte, error) {
	var buf []byte

	buf, err := parser.parseBytes(2)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 0-1 of header")
	}
	exSignature := [2]byte{buf[0], buf[1]}

	_, err = parser.parseBytes(4)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 2-5 of header")
	}

	relocationItems, err := parser.parseWord()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 6-7 of header")
	}

	exHeaderSize, err := parser.parseWord()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 8-9 of header")
	}

	_, err = parser.parseBytes(4)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 10-13 of header")
	}

	exInitSS, err := parser.parseWord()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 14-15 of header")
	}

	exInitSP, err := parser.parseWord()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 16-17 of header")
	}

	_, err = parser.parseBytes(2)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 18-19 of header")
	}

	exInitIP, err := parser.parseWord()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 20-21 of header")
	}

	exInitCS, err := parser.parseWord()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 22-23 of header")
	}

	relocationTableOffset, err := parser.parseWord()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse bytes at 24-25 of header")
	}

	remainHeaderBytes := int(exHeaderSize) * paragraphSize - int(parser.offset)
	_, err = parser.parseBytes(remainHeaderBytes)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse remains of header")
	}

	loadModule, err := parser.parseRemains()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse load module")
	}

	return &header{
		exSignature: exSignature,
		relocationItems: relocationItems,
		exHeaderSize: exHeaderSize,
		exInitSS: exInitSS,
		exInitSP: exInitSP,
		exInitIP: exInitIP,
		exInitCS: exInitCS,
		relocationTableOffset: relocationTableOffset,
	}, loadModule, nil
}

// --- memory

type memory struct {
	loadModule []byte
	memorySize int
}
type address uint16

// Prepare memory whose size is same as load module
func newMemory(loadModule []byte) *memory {
	loadModuleSize := len(loadModule)
	m := make([]byte, loadModuleSize)
	for i := 0; i < loadModuleSize; i++ {
		m[i] = loadModule[i]
	}
	return &memory{loadModule: m, memorySize: loadModuleSize}
}

// TODO: How to calculate the necessary stack size?
func newMemoryFromHeader(loadModule []byte, header *header) *memory {
	loadModuleSize := len(loadModule)
	stackSize := int(realAddress(header.exInitSS, header.exInitSP))
	memorySize := loadModuleSize + stackSize
	m := make([]byte, memorySize)
	for i := 0; i < loadModuleSize; i++ {
		m[i] = loadModule[i]
	}
	return &memory{loadModule: m, memorySize: memorySize}
}

func (memory *memory) readBytes(at address, n int) ([]byte, error) {
	if int(at) + (n-1) >= memory.memorySize {
		return nil, fmt.Errorf("illegal address: 0x%05x", at)
	}

	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		buf[i] = memory.loadModule[int(at)+i]
	}
	return buf, nil
}

func (memory *memory) readByte(at address) (byte, error) {
	b, err := memory.readBytes(at, 1)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read byte")
	}
	return b[0], nil
}

func (memory *memory) readWord(at address) (word, error) {
	buf, err := memory.readBytes(at, 2)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read word")
	}
	return word(buf[1]) << 8 + word(buf[0]), nil
}

func (memory *memory) readInt8(at address) (int8, error) {
	var v int8
	bs, err := memory.readBytes(at, 1)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read int8")
	}
	buf := bytes.NewReader(bs)
	err = binary.Read(buf, binary.LittleEndian, &v)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse as int8")
	}
	return v, nil
}

func (memory *memory) readInt16(at address) (int16, error) {
	var v int16
	bs, err := memory.readBytes(at, 2)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read int16")
	}
	buf := bytes.NewReader(bs)
	err = binary.Read(buf, binary.LittleEndian, &v)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse as int16")
	}
	return v, nil
}

func (memory *memory) writeByte(at address, b byte) error {
	if int(at) >= memory.memorySize {
		return fmt.Errorf("illegal address: 0x%05x", at)
	}
	memory.loadModule[at] = b
	return nil
}

func (memory *memory) writeWord(at address, w word) error {
	if int(at) >= memory.memorySize {
		return fmt.Errorf("illegal address: 0x%05x", at)
	}
	low := byte(w & 0x00ff)
	high := byte((w & 0xff00) >> 8)
	memory.loadModule[at] = low
	memory.loadModule[at+1] = high
	return nil
}

// --- registers

type registerW uint8
type registerB uint8
type registerS uint8

const (
	// ref2. 3.1.1.1
	AX = registerW(0)
	CX = registerW(1)
	DX = registerW(2)
	BX = registerW(3)
	SP = registerW(4)
	BP = registerW(5)
	SI = registerW(6)
	DI = registerW(7)
	AL = registerB(0)
	CL = registerB(1)
	DL = registerB(2)
	BL = registerB(3)
	AH = registerB(4)
	CH = registerB(5)
	DH = registerB(6)
	BH = registerB(7)
	ES = registerS(0)
	CS = registerS(1)
	SS = registerS(2)
	DS = registerS(3)
	FS = registerS(4)
	GS = registerS(5)
)

func toRegisterW(x uint8) (registerW, error) {
	switch x {
	case 0:
		return AX, nil
	case 1:
		return CX, nil
	case 2:
		return DX, nil
	case 3:
		return BX, nil
	case 4:
		return SP, nil
	case 5:
		return BP, nil
	case 6:
		return SI, nil
	case 7:
		return DI, nil
	default:
		return 0, errors.Errorf("illegal number for registerW: %d", x)
	}
}

func toRegisterB(x uint8) (registerB, error) {
	switch x {
	case 0:
		return AL, nil
	case 1:
		return CL, nil
	case 2:
		return DL, nil
	case 3:
		return BL, nil
	case 4:
		return AH, nil
	case 5:
		return CH, nil
	case 6:
		return DH, nil
	case 7:
		return BH, nil
	default:
		return 0, errors.Errorf("illegal number for registerW: %d", x)
	}
}

func toRegisterS(x uint8) (registerS, error) {
	switch x {
	case 0:
		return ES, nil
	case 1:
		return CS, nil
	case 2:
		return SS, nil
	case 3:
		return DS, nil
	case 4:
		return FS, nil
	case 5:
		return GS, nil
	default:
		return 0, errors.Errorf("illegal number for registerS:%d", x)
	}
}

type instInt struct {
	operand uint8
}

type instMov struct {
	dest registerW
	imm word
}

type instMovReg8Imm8 struct {
	dest registerB
	imm8 uint8
}

type instMovRegReg struct {
	dest registerW
	src registerW
}

type instMovSRegReg struct {
	dest registerS
	src registerW
}

type instMovRegMemBP struct {
	dest registerW
	disp int8
}

type instMovReg8MemDisp8 struct {
	dest registerB
	base registerW
	disp8 int8
}

type instShl struct {
	register registerW
	imm uint8
}

type instAdd struct {
	dest registerW
	imm uint8
}

type instSub struct {
	dest registerW
	imm uint8
}

type instLea struct {
	dest registerW
	address word
}

type instLeaReg16Disp8 struct {
	dest registerW
	base registerW
	disp8 int8
}

type instPush struct {
	src registerW
}

type instPop struct {
	dest registerW
}

type instCall struct {
	rel int16
}

type instRet struct {

}

type instJmpRel16 struct {
	rel int16
}

type instSti struct {

}

type instAndReg8Imm8 struct {
	reg registerB
	imm8 uint8
}

type instAndMem8Reg8 struct {
	offset word
	reg8 registerB
}

type instMovMem16Reg16 struct {
	offset word
	src registerW
}

type instMovMem8Reg8 struct {
	offset word
	src registerB
}

type instMovReg16Mem16 struct {
	dest registerW
	offset word
}

type instMovMem16Sreg struct {
	offset word
	src registerS
}

type instAddReg16Reg16 struct {
	dest registerW
	src registerW
}

type instShrReg16Imm struct {
	reg registerW
	imm word
}

type instShlReg16Imm struct {
	reg registerW
	imm word
}

type instCmpMem8Imm8 struct {
	offset word
	imm8 int8
}

type instCmpMem16Imm8 struct {
	offset word
	imm8 int8
}

type instCmpReg8Imm8 struct {
	reg8 registerB
	imm8 int8
}

type instCmpReg16Reg16 struct {
	first registerW
	second registerW
}

type instJneRel8 struct {
	rel8 int8
}

type instMovReg16Sreg struct {
	dest registerW
	src registerS
}

type instSubReg16Reg16 struct {
	dest registerW
	src registerW
}

type instSubReg8Reg8 struct {
	dest registerB
	src registerB
}

type instJb struct {
	rel8 int8
}

type instCld struct {

}

type instRepeScasb struct {

}

type instRepMovsb struct {

}

type instRepStosb struct {

}

type instStosb struct {

}

type instJeRel8 struct {
	rel8 int8
}

type instInc struct {
	dest registerW
}

type instDec struct {
	dest registerW
}

type instXorReg16Reg16 struct {
	dest registerW
	src registerW
}

func decodeModRegRM(at address, memory *memory) (byte, byte, registerW, error) {
	buf, err := memory.readByte(at)
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed to parse byte")
	}

	mod := (buf & 0xc0) >> 6     // 0b11000000
	reg := (buf & 0x38) >> 3     // 0b00111000
	rm  := registerW(buf & 0x07) // 0b00000111

	return mod, reg, rm, nil
}

// assume that reader for load module is passed
// inst, read bytes, error
func decodeInst(reader io.Reader) (interface{}, int, *segmentOverride, error) {
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, 0, nil, err
	}
	memory := newMemory(bytes)
	return decodeInstWithMemory(0, memory)
}

// inst, read bytes, register overriding, error
func decodeInstWithMemory(initialAddress address, memory *memory) (interface{}, int, *segmentOverride, error) {
	var inst interface{}
	currentAddress := initialAddress

	rawOpcode, err := memory.readByte(currentAddress)
	currentAddress++
	if err != nil {
		return inst, -1, nil, errors.Wrap(err, "failed to parse opcode")
	}

	switch rawOpcode {
	// add r16,r/m16
	// 03 /r
	case 0x03:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			dest, err := toRegisterW(uint8(reg))
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg value for registerW: %d", reg)
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal rm value for registerW: %d", rm)
			}
			inst = instAddReg16Reg16{dest: dest, src: src}
		} else {
			return inst, -1, nil, errors.Errorf("unknown or not implemented for mod %d", mod)
		}

	// and r/m64,r8
	// 20 /r
	case 0x20:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 0:
			reg8, err := toRegisterB(uint8(reg))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "illegal reg value")
			}
			switch rm {
			case 6:
				offset, err := memory.readWord(currentAddress)
				currentAddress += 2
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse word")
				}
				inst = instAndMem8Reg8{offset: offset, reg8: reg8}
			default:
				return inst, -1, nil, errors.Errorf("unknown or not implemented for rm: %d", rm)
			}
		default:
			return inst, -1, nil, errors.Errorf("unknown or not implemented for mod: %d", mod)
		}

	// segment override by ES
	case 0x26:
		inst, readBytes, _, err := decodeInstWithMemory(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode")
		}
		return inst, readBytes + int(currentAddress - initialAddress), &segmentOverride{sreg: ES}, nil

	// sub r8,r/m8
	// 2a /r
	case 0x2a:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 3:
			dest, err := toRegisterB(reg)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerB")
			}
			src, err := toRegisterB(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerB")
			}
			inst = instSubReg8Reg8{dest: dest, src: src}
		default:
			return inst, -1, nil, errors.Errorf("unknown or not implemented for mod %d", mod)
		}

	// sub r16,r/m16
	// 2b /r
	case 0x2b:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 3:
			dest, err := toRegisterW(reg)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerW")
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerW")
			}
			inst = instSubReg16Reg16{dest: dest, src: src}
		default:
			return inst, -1, nil, errors.Errorf("unknown or not implemented for mod %d", mod)
		}

	// xor r16,r/m16
	// 33 /r
	case 0x33:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 3:
			dest, err := toRegisterW(reg)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerW")
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerW")
			}
			inst = instXorReg16Reg16{dest: dest, src: src}
		default:
			return inst, -1, nil, errors.Errorf("unknown or not implemented for mod %d", mod)
		}

	// cmp r16,r/m16
	// 3b /r
	case 0x3b:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 3:
			dest, err := toRegisterW(reg)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerW")
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerW")
			}
			inst = instCmpReg16Reg16{first: dest, second: src}
		default:
			return inst, -1, nil, errors.Errorf("unknown or not implemented for mod %d", mod)
		}

	// cmp al,imm8
	// 3c ib
	case 0x3c:
		imm8, err := memory.readInt8(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse as int8")
		}
		inst = instCmpReg8Imm8{reg8: AL, imm8: imm8}

	// inc ax
	case 0x40:
		inst = instInc{dest: AX}
	// inc cx
	case 0x41:
		inst = instInc{dest: CX}
	// inc dx
	case 0x42:
		inst = instInc{dest: DX}
	// inc bx
	case 0x43:
		inst = instInc{dest: BX}
	// inc sp
	case 0x44:
		inst = instInc{dest: SP}
	// inc bp
	case 0x45:
		inst = instInc{dest: BP}
	// inc si
	case 0x46:
		inst = instInc{dest: SI}
	// inc di
	case 0x47:
		inst = instInc{dest: DI}

	// dec ax
	case 0x48:
		inst = instDec{dest: AX}
	// dec cx
	case 0x49:
		inst = instDec{dest: CX}
	// dec dx
	case 0x4a:
		inst = instDec{dest: DX}
	// dec bx
	case 0x4b:
		inst = instDec{dest: BX}
	// dec sp
	case 0x4c:
		inst = instDec{dest: SP}
	// dec bp
	case 0x4d:
		inst = instDec{dest: BP}
	// dec si
	case 0x4e:
		inst = instDec{dest: SI}
	// dec di
	case 0x4f:
		inst = instDec{dest: DI}

	// push ax
	case 0x50:
		inst = instPush{src: AX}
	// push cx
	case 0x51:
		inst = instPush{src: CX}
	// push dx
	case 0x52:
		inst = instPush{src: DX}
	// push bx
	case 0x53:
		inst = instPush{src: BX}
	// push sp
	case 0x54:
		inst = instPush{src: SP}
	// push bp
	case 0x55:
		inst = instPush{src: BP}
	// push si
	case 0x56:
		inst = instPush{src: SI}
	// push di
	case 0x57:
		inst = instPush{src: DI}

	// pop ax
	case 0x58:
		inst = instPop{dest: AX}
	// pop cx
	case 0x59:
		inst = instPop{dest: CX}
	// pop dx
	case 0x5a:
		inst = instPop{dest: DX}
	// pop bx
	case 0x5b:
		inst = instPop{dest: BX}
	// pop sp
	case 0x5c:
		inst = instPop{dest: SP}
	// pop bp
	case 0x5d:
		inst = instPop{dest: BP}
	// pop si
	case 0x5e:
		inst = instPop{dest: SI}
	// pop di
	case 0x5f:
		inst = instPop{dest: DI}

	case 0x72:
		offset, err := memory.readInt8(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}
		inst = instJb{rel8: offset}

	// je rel8
	// 74 cb
	case 0x74:
		imm8, err := memory.readInt8(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}
		inst = instJeRel8{rel8: imm8}

	// jne rel8
	// 75 cb
	case 0x75:
		imm8, err := memory.readInt8(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}
		inst = instJneRel8{rel8: imm8}

	case 0x80:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch reg {
		// and r/m8, imm8
		case 4:
			if mod != 3 {
				return nil, -1, nil, errors.Errorf("expect mod is %d but %d", 3, mod)
			}

			imm, err := memory.readByte(currentAddress)
			currentAddress++
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
			}

			regB, err := toRegisterB(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "unknown register")
			}

			inst = instAndReg8Imm8{reg: regB, imm8: imm}

		// cmp r/m8,imm8
		// 80 /7 ib
		case 7:
			if mod != 0 {
				return nil, -1, nil, errors.Errorf("expected mod is %d but %d", 0, mod)
			}

			if rm != 6 {
				return nil, -1, nil, errors.Errorf("expected rm is %d but %d", 6, rm)
			}

			word, err := memory.readWord(currentAddress)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse word")
			}
			currentAddress += 2

			imm, err := memory.readInt8(currentAddress)
			currentAddress++
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
			}

			inst = instCmpMem8Imm8{offset: word, imm8: imm}

		default:
			return nil, -1, nil, errors.Errorf("unknown reg: %d", reg)
		}

	// add r/m16, imm8
	// 83 /5 -> sub r/m16, imm8
	// 83 /7 ib ->  cmp r/m16,imm8
	case 0x83:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch reg {
		// add
		case 0:
			if mod != 3 {
				return nil, -1, nil, errors.Errorf("expect mod is 0b11 but %02b", mod)
			}
			imm, err := memory.readByte(currentAddress)
			currentAddress++
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
			}

			switch rm {
			case AX:
				inst = instAdd{dest: AX, imm: imm}
			case CX:
				inst = instAdd{dest: CX, imm: imm}
			case DX:
				inst = instAdd{dest: DX, imm: imm}
			case BX:
				inst = instAdd{dest: BX, imm: imm}
			case SP:
				inst = instAdd{dest: SP, imm: imm}
			default:
				return nil, -1, nil, errors.Errorf("unknown register: %d", rm)
			}
		// sub
		case 5:
			if mod != 3 {
				return nil, -1, nil, errors.Errorf("expect mod is 0b11 but %02b", mod)
			}
			imm, err := memory.readByte(currentAddress)
			currentAddress++
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to decode sub inst")
			}
			switch rm {
			case AX:
				inst = instSub{dest: AX, imm: imm}
			case DX:
				inst = instSub{dest: DX, imm: imm}
			case CX:
				inst = instSub{dest: CX, imm: imm}
			case SP:
				inst = instSub{dest: SP, imm: imm}
			default:
				return nil, -1, nil, errors.Errorf("unknown register: %d", rm)
			}
		// cmp
		case 7:
			switch mod {
			case 0:
				offset, err := memory.readWord(currentAddress)
				currentAddress += 2
				if err != nil {
					return nil, -1, nil, errors.Wrap(err, "failed to parse disp16")
				}
				imm8, err := memory.readInt8(currentAddress)
				currentAddress++
				if err != nil {
					return nil, -1, nil, errors.Wrap(err, "failed to parse imm8")
				}
				inst = instCmpMem16Imm8{offset: offset, imm8: imm8}
			default:
				return nil, -1, nil, errors.Errorf("not yet implemented for mod: %d", mod)
			}
		default:
			return nil, -1, nil, errors.Errorf("expect reg is 0 but %d", reg)
		}

	// 88 /r
	// mov r/m8,r8
	case 0x88:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 0 {
			src, err := toRegisterB(reg)
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg vlue for registerB: %d", reg)
			}

			switch rm {
			case 6:
				offset, err := memory.readWord(currentAddress)
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse word")
				}
				currentAddress += 2
				inst = instMovMem8Reg8{offset: offset, src: src}
			default:
				return inst, -1, nil, errors.Errorf("not yet implmeneted for rm %d", rm)
			}
		} else {
			return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// 89 /r
	// mov r/m16,r16
	case 0x89:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 0 {
			src, err := toRegisterW(reg)
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg vlue for registerW: %d", reg)
			}

			switch rm {
			case 6:
				offset, err := memory.readWord(currentAddress)
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse word")
				}
				currentAddress += 2
				inst = instMovMem16Reg16{offset: offset, src: src}
			default:
				return inst, -1, nil, errors.Errorf("not yet implmeneted for rm %d", rm)
			}
		} else {
			return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// mov r8,r/m8
	// 8A /r
	case 0x8a:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 1:
			dest, err := toRegisterB(reg)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse as registerB")
			}
			switch rm {
			case 5:
				disp8, err := memory.readInt8(currentAddress)
				currentAddress++
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to read int8 on memory")
				}

				inst = instMovReg8MemDisp8{dest: dest, base: DI, disp8: disp8}
			default:
				return inst, -1, nil, errors.Errorf("not yet implemented for rm %d", rm)
			}

		default:
			return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// 8b /r (/r indicates that the ModR/M byte of the instruction contains a register operand and an r/m operand)
	// mov r16,r/m16
	case 0x8b:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			dest, err := toRegisterW(uint8(reg))
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg value for registerW: %d", reg)
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal rm value for registerW: %d", rm)
			}
			inst = instMovRegReg{dest: dest, src: src}
		} else if mod == 1 {
			dest, err := toRegisterW(uint8(reg))
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg value for registerW: %d", reg)
			}
			switch (rm) {
			case 6:
				disp, err := memory.readInt8(currentAddress)
				currentAddress++
				if err != nil {
					return inst, -1, nil, errors.Errorf("failed to read as int8")
				}
				inst = instMovRegMemBP{dest: dest, disp: disp}
			default:
				return inst, -1, nil, errors.Errorf("not yet implemented for rm %d", rm)
			}
		} else if mod == 0 {
			dest, err := toRegisterW(reg)
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg vlue for registerW: %d", reg)
			}

			switch rm {
			case 6:
				offset, err := memory.readWord(currentAddress)
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse word")
				}
				currentAddress += 2
				inst = instMovReg16Mem16{dest: dest, offset: offset}
			default:
				return inst, -1, nil, errors.Errorf("not yet implmeneted for rm %d", rm)
			}
		} else {
			return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// 8c /r
	// mov r/m16,Sreg
	case 0x8c:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 0:
			src, err := toRegisterS(reg)
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg vlue for registerS: %d", reg)
			}

			switch rm {
			case 6:
				offset, err := memory.readWord(currentAddress)
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse word")
				}
				currentAddress += 2
				inst = instMovMem16Sreg{offset: offset, src: src}
			default:
				return inst, -1, nil, errors.Errorf("not yet implmeneted for rm %d", rm)
			}

		case 3:
			sreg, err := toRegisterS(uint8(reg))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to parse sreg")
			}
			inst = instMovReg16Sreg{dest: rm, src: sreg}

		default:
			return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// lea r16,m
	// 8d /r
	case 0x8d:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch mod {
		case 0:
			switch rm {
			case 6:
				dest, err := toRegisterW(reg)
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "illegal reg value for registerW")
				}
				address, err := memory.readWord(currentAddress)
				currentAddress += 2
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse address of lea")
				}
				inst = instLea{dest: dest, address: address}
			default:
				return inst, -1, nil, errors.Errorf("not yet implemented for mod: %d", mod)
			}
		case 1:
			dest, err := toRegisterW(uint8(reg))
			if err != nil {
				return inst, -1, nil, errors.Errorf("failed to parse as registerW")
			}
			disp8, err := memory.readInt8(currentAddress)
			currentAddress++
			switch rm {
			case 5:
				inst = instLeaReg16Disp8{dest: dest, base: DI, disp8: disp8}
			default:
				return inst, -1, nil, errors.Errorf("not yet implemented for rm: %d", rm)
			}
		default:
			return inst, -1, nil, errors.Errorf("not yet implemented for mod: %d", mod)
		}

	// 8e /r
	// mov Sreg,r/m16
	// Sreg ES=0, CS=1, SS=2, DS=3, FS=4, GS=5
	case 0x8e:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			dest, err := toRegisterS(reg)
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg value for registerS: %d", reg)
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, nil, errors.Errorf("illegal reg value for registerW: %d", rm)
			}
			inst = instMovSRegReg{dest: dest, src: src}
		} else {
			return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// mov ax,moffs16
	// A1
	case 0xa1:
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm16")
		}
		inst = instMovReg16Mem16{dest: AX, offset: imm}

	// mov moffs8,al
	// A2
	case 0xa2:
		offset, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm16")
		}
		inst = instMovMem8Reg8{offset: offset, src: AL}

	// mov moffs16,ax
	// A3
	case 0xa3:
		offset, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm16")
		}
		inst = instMovMem16Reg16{offset: offset, src: AX}

	// stosb
	case 0xaa:
		inst = instStosb{}

	// b0+ rb ib
	// mov r8,imm8
	case 0xb0:
		// al
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: AL, imm8: imm}
	case 0xb1:
		// cl
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: CL, imm8: imm}
	case 0xb2:
		// dl
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: DL, imm8: imm}
	case 0xb3:
		// bl
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: BL, imm8: imm}
	case 0xb4:
		// ah
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: AH, imm8: imm}
	case 0xb5:
		// ch
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: CH, imm8: imm}
	case 0xb6:
		// dh
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: DH, imm8: imm}
	case 0xb7:
		// bh
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMovReg8Imm8{dest: BH, imm8: imm}

	// b8+ rw iw
	// mov r16,imm16
	case 0xb8:
		// ax
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: AX, imm: imm}
	case 0xb9:
		// cx
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: CX, imm: imm}
	case 0xba:
		// dx
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: DX, imm: imm}
	case 0xbb:
		// bx
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: BX, imm: imm}
	case 0xbc:
		// sp
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: SP, imm: imm}
	case 0xbd:
		// bp
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: BP, imm: imm}
	case 0xbe:
		// si
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: SI, imm: imm}
	case 0xbf:
		// di
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		inst = instMov{dest: DI, imm: imm}

	// shl r/m16,imm8
	// FIXME: handle memory address as source
	case 0xc1:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod != 3 {
			return nil, -1, nil, errors.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 4 {
			return nil, -1, nil, errors.Errorf("expect reg is /4 but %d", reg)
		}

		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return nil, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}

		switch rm {
		case AX:
			inst = instShl{register: AX, imm: imm}
		case CX:
			inst = instShl{register: CX, imm: imm}
		default:
			return nil, -1, nil, errors.Errorf("unknown register: %d", rm)
		}

	// ret (near return)
	case 0xc3:
		inst = instRet{}

	// int imm8
	case 0xcd:
		operand, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse operand")
		}
		inst = instInt{operand: operand}

	// shr dx,1
	// d1 /4
	// shr dx,1
	// d1 /5
	case 0xd1:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			switch reg {
			case 4:
				reg, err := toRegisterW(uint8(rm))
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse registerW")
				}
				inst = instShlReg16Imm{reg: reg, imm: 1}
			case 5:
				reg, err := toRegisterW(uint8(rm))
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse registerW")
				}
				inst = instShrReg16Imm{reg: reg, imm: 1}
			default:
				return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
			}
		} else {
			return inst, -1, nil, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// call rel16
	case 0xe8:
		rel, err := memory.readInt16(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse int16")
		}
		inst = instCall{rel: rel}

	// jmp rel16
	case 0xe9:
		rel, err := memory.readInt16(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse int16")
		}
		inst = instJmpRel16{rel: rel}

	case 0xf3:
		stringOperation, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse stringOperation")
		}
		switch stringOperation {
		case 0xa4:
			// rep movsb
			inst = instRepMovsb{}
		case 0xaa:
			// rep stosb
			inst = instRepStosb{}
		case 0xae:
			// repe scasb
			inst = instRepeScasb{}
		default:
			return inst, -1, nil, errors.Errorf("not yet implemented string instruction")
		}

	// sti
	case 0xfb:
		inst = instSti{}

	// cld
	case 0xfc:
		inst = instCld{}

	default:
		return inst, -1, nil, errors.Errorf("unknown opcode: 0x%02x", rawOpcode)
	}
	return inst, int(currentAddress - initialAddress), nil, nil
}

// for int 21
type intHandler func(*state, *memory) error
type intHandlers map[uint8]intHandler

func intHandler30(s *state, memory *memory) error {
	// do nothing for now
	return nil
}

func intHandler4a(s *state, memory *memory) error {
	// do nothing for now
	return nil
}

func intHandler4c(s *state, memory *memory) error {
	s.exitCode = exitCode(s.al())
	s.shouldExit = true
	return nil
}

// DS:DX has the address of string
// string should be ended with '$'
func intHandler09(s *state, memory *memory) error {
	var bs []byte
	startAddress := s.realAddress(s.ds, s.dx)
	for {
		b, err := memory.readByte(startAddress)
		if err != nil {
			return err
		}
		if b == '$' {
			break
		}
		bs = append(bs, b)
		startAddress++
	}
	fmt.Print(string(bs))
	return nil
}

// FIXME: Type general registers, segment registers respectively
type state struct {
	ax, cx, dx, bx, sp, bp, si, di, ss, cs, ip, ds, es word
	eflags dword
	exitCode                   exitCode
	shouldExit                 bool
	intHandlers                intHandlers
}

const (
	EFLAGS_ZF     = 0x00000040
	EFLAGS_ZF_INV = 0xffffffbf
	EFLAGS_CF     = 0x00000001
	EFLAGS_CF_INV = 0xfffffffe
	EFLAGS_DF     = 0x00000200
	EFLAGS_DF_INV = 0xfffffdff
)

func newState(header *header, customIntHandlers intHandlers) state {
	// --- Prepare interrupted handlers

	intHandlers := make(intHandlers)
	for k, v := range customIntHandlers {
		intHandlers[k] = v
	}

	// int 21 30h
	if _, ok := intHandlers[0x30]; !ok {
		intHandlers[0x30] = intHandler30
	}

	// int 21 4ah
	if _, ok := intHandlers[0x4a]; !ok {
		intHandlers[0x4a] = intHandler4a
	}

	// int 21 4ch
	if _, ok := intHandlers[0x4c]; !ok {
		intHandlers[0x4c] = intHandler4c
	}

	// int 21 09h
	if _, ok := intHandlers[0x09]; !ok {
		intHandlers[0x09] = intHandler09
	}

	return state{
		sp: header.exInitSP,
		ss: header.exInitSS,
		ip: header.exInitIP,
		cs: header.exInitCS,
		intHandlers: intHandlers}
}

func (s state) al() uint8 {
	return uint8(s.ax & 0x00ff)
}

func (s state) cl() uint8 {
	return uint8(s.cx & 0x00ff)
}

func (s state) dl() uint8 {
	return uint8(s.dx & 0x00ff)
}

func (s state) bl() uint8 {
	return uint8(s.bx & 0x00ff)
}

func (s state) ah() uint8 {
	return uint8(s.ax >> 8)
}

func (s state) ch() uint8 {
	return uint8(s.cx >> 8)
}

func (s state) dh() uint8 {
	return uint8(s.dx >> 8)
}

func (s state) bh() uint8 {
	return uint8(s.bx >> 8)
}

func (s state) realAddress(sreg word, reg word) address {
	return realAddress(sreg, reg)
}

func (s state) realIP() address {
	return address(s.cs << 4 + s.ip)
}

// return true if zf == 1
func (s state) isActiveZF() bool {
	zf := s.eflags & EFLAGS_ZF
	return zf != 0
}

// return true if zf == 0
func (s state) isNotActiveZF() bool {
	return !s.isActiveZF()
}

func (s state) setZF() state {
	s.eflags = s.eflags | EFLAGS_ZF
	return s
}

func (s state) resetZF() state {
	s.eflags = s.eflags & EFLAGS_ZF_INV
	return s
}

// return true if cf == 1
func (s state) isActiveCF() bool {
	cf := s.eflags & EFLAGS_CF
	return cf != 0
}

// return true if cf == 0
func (s state) isNotActiveCF() bool {
	return !s.isActiveCF()
}

func (s state) setCF() state {
	s.eflags = s.eflags | EFLAGS_CF
	return s
}

func (s state) resetCF() state {
	s.eflags = s.eflags & EFLAGS_CF_INV
	return s
}

// return true if df == 1
func (s state) isActiveDF() bool {
	df := s.eflags & EFLAGS_DF
	return df != 0
}

// return true if df == 0
func (s state) isNotActiveDF() bool {
	return !s.isActiveDF()
}

func (s state) setDF() state {
	s.eflags = s.eflags | EFLAGS_DF
	return s
}

func (s state) resetDF() state {
	s.eflags = s.eflags & EFLAGS_DF_INV
	return s
}

func (s state) readWordGeneralReg(r registerW) (word, error) {
	switch r {
	case AX:
		return s.ax, nil
	case CX:
		return s.cx, nil
	case DX:
		return s.dx, nil
	case BX:
		return s.bx, nil
	case SP:
		return s.sp, nil
	case BP:
		return s.bp, nil
	case SI:
		return s.si, nil
	case DI:
		return s.di, nil
	default:
		return 0, errors.Errorf("illegal registerW or not implemented: %d", r)
	}
}

func (s state) readByteGeneralReg(r registerB) (uint8, error) {
	switch r {
	case AL:
		return s.al(), nil
	case CL:
		return s.cl(), nil
	case DL:
		return s.dl(), nil
	case BL:
		return s.bl(), nil
	case AH:
		return s.ah(), nil
	case CH:
		return s.ch(), nil
	case DH:
		return s.dh(), nil
	case BH:
		return s.bh(), nil
	default:
		return 0, errors.Errorf("illegal registerB or not implemented: %d", r)
	}
}

func (s state) readWordSreg(r registerS) (word, error) {
	switch r {
	case ES:
		return s.es, nil
	case CS:
		return s.cs, nil
	case SS:
		return s.ss, nil
	case DS:
		return s.ds, nil
		/*
	case FS:
		return s.fs, nil
	case GS:
		return s.gs, nil
		*/
	default:
		return 0, errors.Errorf("illegal number for registerS:%d", r)
	}
}

func (s state) writeByteGeneralReg(r registerB, b uint8) (state, error) {
	switch r {
	case AL:
		s.ax = (s.ax & 0xff00) | word(b)
	case CL:
		s.cx = (s.cx & 0xff00) | word(b)
	case DL:
		s.dx = (s.dx & 0xff00) | word(b)
	case BL:
		s.bx = (s.bx & 0xff00) | word(b)
	case AH:
		s.ax = (s.ax & 0x00ff) | word(b << 8)
	case CH:
		s.cx = (s.cx & 0x00ff) | word(b << 8)
	case DH:
		s.dx = (s.dx & 0x00ff) | word(b << 8)
	case BH:
		s.bx = (s.bx & 0x00ff) | word(b << 8)
	default:
		return s, errors.Errorf("illegal number for registerB: %d", r)
	}
	return s, nil
}

func (s state) writeWordGeneralReg(r registerW, w word) (state, error) {
	switch r {
	case AX:
		s.ax = w
		return s, nil
	case CX:
		s.cx = w
		return s, nil
	case DX:
		s.dx = w
		return s, nil
	case BX:
		s.bx = w
		return s, nil
	case SP:
		s.sp = w
		return s, nil
	case BP:
		s.bp = w
		return s, nil
	case SI:
		s.si = w
		return s, nil
	case DI:
		s.di = w
		return s, nil
	default:
		return s, errors.Errorf("illegal registerW or not implemented: %d", r)
	}
}

func (s state) pushWord(w word, memory *memory) (state, error) {
	s.sp -= 2
	err := memory.writeWord(s.realAddress(s.ss, s.sp), w)
	if err != nil {
		return s, errors.Wrap(err, "failed to push word")
	}
	return s, nil
}

func (s state) popWord(memory *memory) (word, state, error) {
	w, err := memory.readWord(s.realAddress(s.ss, s.sp))
	if err != nil {
		return 0, s, errors.Wrap(err, "failed in execPop")
	}
	s.sp += 2
	return w, s, nil
}

func execMov(inst instMov, state state) (state, error) {
	switch inst.dest {
	case AX:
		state.ax = inst.imm
	case CX:
		state.cx = inst.imm
	case DX:
		state.dx = inst.imm
	case BX:
		state.bx = inst.imm
	case SP:
		state.sp = inst.imm
	case BP:
		state.bp = inst.imm
	case SI:
		state.si = inst.imm
	case DI:
		state.di = inst.imm
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execMovReg8Imm8(inst instMovReg8Imm8, state state) (state, error) {
	switch inst.dest {
	case AL:
		state.ax = word(uint16(inst.imm8) + (0xff00) & uint16(state.ax))
	case CL:
		state.cx = word(uint16(inst.imm8) + (0xff00) & uint16(state.cx))
	case DL:
		state.dx = word(uint16(inst.imm8) + (0xff00) & uint16(state.dx))
	case BL:
		state.bx = word(uint16(inst.imm8) + (0xff00) & uint16(state.bx))
	case AH:
		state.ax = word((uint16(inst.imm8) << 8) + (0x00ff) & uint16(state.ax))
	case CH:
		state.cx = word((uint16(inst.imm8) << 8) + (0x00ff) & uint16(state.cx))
	case DH:
		state.dx = word((uint16(inst.imm8) << 8) + (0x00ff) & uint16(state.dx))
	case BH:
		state.bx = word((uint16(inst.imm8) << 8) + (0x00ff) & uint16(state.bx))
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execMovRegReg(inst instMovRegReg, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed to get reg")
	}
	switch inst.dest {
	case AX:
		state.ax = v
	case CX:
		state.cx = v
	case DX:
		state.dx = v
	case BX:
		state.bx = v
	case SP:
		state.sp = v
	case BP:
		state.bp = v
	case SI:
		state.si = v
	case DI:
		state.di = v
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execMovSRegReg(inst instMovSRegReg, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed to get reg")
	}
	switch inst.dest {
	case ES:
		state.es = v
	case SS:
		state.ss = v
	case DS:
		state.ds = v
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execMovRegMemBP(inst instMovRegMemBP, state state, memory *memory) (state, error) {
	base, err := state.readWordGeneralReg(BP)
	if err != nil {
		return state, errors.Wrap(err, "failed to get reg")
	}
	at := address(int(state.realAddress(state.ss, base)) + int(inst.disp))
	v, err := memory.readWord(at)
	if err != nil {
		return state, errors.Wrap(err, "failed to read memory")
	}
	state, err = state.writeWordGeneralReg(inst.dest, v)
	if err != nil {
		return state, errors.Wrap(err, "failed in moving to reg")
	}
	return state, nil
}

func execMovReg8MemDisp8(inst instMovReg8MemDisp8, state state, memory *memory) (state, error) {
	base, err := state.readWordGeneralReg(inst.base)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovReg8MemDisp8")
	}
	at := address(int(state.realAddress(state.ds, base)) + int(inst.disp8))
	v, err := memory.readByte(at)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovReg8MemDisp8")
	}
	state, err = state.writeByteGeneralReg(inst.dest, v)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovReg8MemDisp8")
	}
	return state, nil
}

func execShl(inst instShl, state state) (state, error) {
	switch inst.register {
	case AX:
		state.ax <<= inst.imm
	case CX:
		state.cx <<= inst.imm
	case DX:
		state.dx <<= inst.imm
	case BX:
		state.bx <<= inst.imm
	default:
		return state, errors.Errorf("unknown register: %v", inst.register)
	}
	return state, nil
}

func execAdd(inst instAdd, state state) (state, error) {
	switch inst.dest {
	case AX:
		state.ax += word(inst.imm)
	case CX:
		state.cx += word(inst.imm)
	case DX:
		state.dx += word(inst.imm)
	case BX:
		state.bx += word(inst.imm)
	case SP:
		state.sp += word(inst.imm)
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execSub(inst instSub, state state) (state, error) {
	switch inst.dest {
	case AX:
		state.ax -= word(inst.imm)
	case DX:
		state.dx -= word(inst.imm)
	case CX:
		state.cx -= word(inst.imm)
	case SP:
		state.sp -= word(inst.imm)
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execLea(inst instLea, state state) (state, error) {
	switch inst.dest {
	case AX:
		state.ax = inst.address
	case CX:
		state.cx = inst.address
	case DX:
		state.dx = inst.address
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execInstLeaReg16Disp8(inst instLeaReg16Disp8, state state, memory *memory) (state, error) {
	vBase, err := state.readWordGeneralReg(inst.base)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstLeaReg16Disp8")
	}
	vDS, err := state.readWordSreg(DS)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstLeaReg16Disp8")
	}
	leaVal := word(state.realAddress(vDS, vBase) + address(inst.disp8))
	switch inst.dest {
	case AX:
		state.ax = leaVal
	case CX:
		state.cx = leaVal
	case DX:
		state.dx = leaVal
	case BX:
		state.bx = leaVal
	case SP:
		state.sp = leaVal
	case BP:
		state.bp = leaVal
	case SI:
		state.si = leaVal
	case DI:
		state.di = leaVal
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execInt(inst instInt, state state, memory *memory) (state, error) {
	switch inst.operand {
	case 0x21:
		if handler, ok := state.intHandlers[state.ah()]; ok {
			err := handler(&state, memory)
			if err != nil {
				return state, errors.Wrap(err, "failed in handler")
			}
		} else {
			return state, errors.Errorf("int 21 with unknown value of ax: %04x", state.ax)
		}
	default:
		return state, errors.Errorf("unknown operand: %v", inst.operand)
	}
	return state, nil
}

func execPush(inst instPush, state state, memory *memory) (state, error) {
	v, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPush")
	}
	state, err = state.pushWord(v, memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPush")
	}
	return state, nil
}

func execPop(inst instPop, state state, memory *memory) (state, error) {
	w, state, err := state.popWord(memory)
	state, err = state.writeWordGeneralReg(inst.dest, w)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPop")
	}
	return state, nil
}

func execCall(inst instCall, state state, memory *memory) (state, error) {
	state, err := state.pushWord(state.ip, memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execCall")
	}
	state.ip = word(int16(state.ip) + inst.rel)
	return state, nil
}

func execRet(inst instRet, state state, memory *memory) (state, error) {
	returnAddress, state, err := state.popWord(memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execRet")
	}
	state.ip = returnAddress
	return state, nil
}

func execJmpRel16(inst instJmpRel16, state state, memory *memory) (state, error) {
	state.ip = word(int16(state.ip) + inst.rel)
	return state, nil
}

func execSti(inst instSti, state state, memory *memory) (state, error) {
	// do nothing now
	return state, nil
}

func execAndReg8Imm8(inst instAndReg8Imm8, state state, memory *memory) (state, error) {
	switch inst.reg {
	case AL:
		state.ax &= word(uint16(0xff << 8) + uint16(inst.imm8))
	case CL:
		state.cx &= word(uint16(0xff << 8) + uint16(inst.imm8))
	case DL:
		state.dx &= word(uint16(0xff << 8) + uint16(inst.imm8))
	case BL:
		state.bx &= word(uint16(0xff << 8) + uint16(inst.imm8))
	default:
		return state, errors.Errorf("unknown register: %v", inst.reg)
	}
	return state, nil
}

func execAndMem8Reg8(inst instAndMem8Reg8, state state, memory *memory) (state, error) {
	vReg, err := state.readByteGeneralReg(inst.reg8)
	if err != nil {
		return state, errors.Wrap(err, "failed in execAndMem8Reg8")
	}
	vMem, err := memory.readByte(state.realAddress(state.ds, inst.offset))
	if err != nil {
		return state, errors.Wrap(err, "failed in execAndMem8Reg8")
	}
	res := byte(vReg) & vMem
	state, err = state.writeByteGeneralReg(inst.reg8, res)
	if err != nil {
		return state, errors.Wrap(err, "failed in execAndMem8Reg8")
	}
	return state, nil
}

func execMovMem16Reg16(inst instMovMem16Reg16, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	initDS := state.ds
	if segmentOverride != nil {
		switch segmentOverride.sreg {
		case ES:
			state.ds = state.es
		default:
			return state, errors.Errorf("not yet implemented or illegal sreg: %#v", segmentOverride.sreg)
		}
	}

	v, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovMem16Reg16")
	}

	err = memory.writeWord(realAddress(state.ds, inst.offset), v)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovMem16Reg16")
	}

	state.ds = initDS
	return state, nil
}

func execMovMem8Reg8(inst instMovMem8Reg8, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	initDS := state.ds
	if segmentOverride != nil {
		switch segmentOverride.sreg {
		case ES:
			state.ds = state.es
		default:
			return state, errors.Errorf("not yet implemented or illegal sreg: %#v", segmentOverride.sreg)
		}
	}

	v, err := state.readByteGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovMem8Reg8")
	}

	err = memory.writeByte(realAddress(state.ds, inst.offset), v)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovMem8Reg8")
	}

	state.ds = initDS
	return state, nil
}

func execMovReg16Mem16(inst instMovReg16Mem16, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	initDS := state.ds
	if segmentOverride != nil {
		switch segmentOverride.sreg {
		case ES:
			state.ds = state.es
		default:
			return state, errors.Errorf("not yet implemented or illegal sreg: %#v", segmentOverride.sreg)
		}
	}

	v, err := memory.readWord(realAddress(state.ds, inst.offset))
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovReg16Mem16")
	}

	state, err = state.writeWordGeneralReg(inst.dest, v)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovReg16Mem16")
	}

	state.ds = initDS
	return state, nil
}

func execMovMem16Sreg(inst instMovMem16Sreg, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	initDS := state.ds
	if segmentOverride != nil {
		switch segmentOverride.sreg {
		case ES:
			state.ds = state.es
		default:
			return state, errors.Errorf("not yet implemented or illegal sreg: %#v", segmentOverride.sreg)
		}
	}

	v, err := state.readWordSreg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovMem16Sreg")
	}

	err = memory.writeWord(state.realAddress(state.ds, inst.offset), v)
	if err != nil {
		return state, errors.Wrap(err, "failed in execMovMem16Sreg")
	}

	state.ds = initDS
	return state, nil
}

func execAddReg16Reg16(inst instAddReg16Reg16, state state) (state, error) {
	srcValue, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Errorf("failed in execAddReg16Reg16")
	}
	destValue, err := state.readWordGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Errorf("failed in execAddReg16Reg16")
	}
	state, err = state.writeWordGeneralReg(inst.dest, (srcValue + destValue))
	if err != nil {
		return state, errors.Errorf("failed in execAddReg16Reg16")
	}
	return state, nil
}

func execShrReg16Imm(inst instShrReg16Imm, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.reg)
	if err != nil {
		return state, errors.Wrap(err, "failed in execShrReg16Imm")
	}
	state, err = state.writeWordGeneralReg(inst.reg, v >> 1)
	if err != nil {
		return state, errors.Wrap(err, "failed in execShrReg16Imm")
	}
	return state, nil
}

func execShlReg16Imm(inst instShlReg16Imm, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.reg)
	if err != nil {
		return state, errors.Wrap(err, "failed in execShlReg16Imm")
	}
	state, err = state.writeWordGeneralReg(inst.reg, v << 1)
	if err != nil {
		return state, errors.Wrap(err, "failed in execShlReg16Imm")
	}
	return state, nil
}

func execInstCmpMem8Imm8(inst instCmpMem8Imm8, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	initDS := state.ds
	if segmentOverride != nil {
		switch segmentOverride.sreg {
		case ES:
			state.ds = state.es
		default:
			return state, errors.Errorf("not yet implemented or illegal sreg: %#v", segmentOverride.sreg)
		}
	}

	v, err := memory.readInt8(state.realAddress(state.ds, inst.offset))
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstCmpMem8Imm8")
	}

	if v == inst.imm8 {
		state = state.setZF()
		state = state.resetCF()
	} else if v < inst.imm8 {
		state = state.resetZF()
		state = state.setCF()
	} else {
		state = state.resetZF()
		state = state.resetCF()
	}

	state.ds = initDS
	return state, nil
}

func execInstCmpMem16Imm8(inst instCmpMem16Imm8, state state, memory *memory) (state, error) {
	v, err := memory.readInt16(state.realAddress(state.ds, inst.offset))
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstCmpMem8Imm8")
	}

	if v == int16(inst.imm8) {
		state = state.setZF()
		state = state.resetCF()
	} else if v < int16(inst.imm8) {
		state = state.resetZF()
		state = state.setCF()
	} else {
		state = state.resetZF()
		state = state.resetCF()
	}

	return state, nil
}

func execInstCmpReg8Imm8(inst instCmpReg8Imm8, state state, memory *memory) (state, error) {
	v, err := state.readByteGeneralReg(inst.reg8)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstCmpReg8Imm8")
	}
	if v == uint8(inst.imm8) {
		state = state.setZF()
		state = state.resetCF()
	} else if v < uint8(inst.imm8) {
		state = state.resetZF()
		state = state.setCF()
	} else {
		state = state.resetZF()
		state = state.resetCF()
	}
	return state, nil
}

func execInstCmpReg16Reg16(inst instCmpReg16Reg16, state state) (state, error) {
	firstV, err := state.readWordGeneralReg(inst.first)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg16Reg16")
	}
	secondV, err := state.readWordGeneralReg(inst.second)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg16Reg16")
	}
	if firstV == secondV {
		state = state.setZF()
		state = state.resetCF()
	} else if firstV < secondV {
		state = state.resetZF()
		state = state.setCF()
	} else {
		state = state.resetZF()
		state = state.resetCF()
	}
	return state, nil
}

func execInstJneRel8(inst instJneRel8, state state) (state, error) {
	if state.isNotActiveZF() {
		state.ip = word(int16(state.ip) + int16(inst.rel8))
	}
	return state, nil
}

func execInstMovReg16Sreg(inst instMovReg16Sreg, state state) (state, error) {
	v, err := state.readWordSreg(inst.src)
	if err != nil {
		return state, errors.Errorf("failed in execInstMovReg16Sreg")
	}
	state, err = state.writeWordGeneralReg(inst.dest, v)
	if err != nil {
		return state, errors.Errorf("failed in execInstMovReg16Sreg")
	}
	return state, nil
}

func execInstSubReg16Reg16(inst instSubReg16Reg16, state state) (state, error) {
	srcV, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg16Reg16")
	}
	destV, err := state.readWordGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg16Reg16")
	}
	state, err = state.writeWordGeneralReg(inst.dest, destV - srcV)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg16Reg16")
	}
	return state, nil
}

func execInstSubReg8Reg8(inst instSubReg8Reg8, state state) (state, error) {
	srcV, err := state.readByteGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg8Reg8")
	}
	destV, err := state.readByteGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg8Reg8")
	}
	state, err = state.writeByteGeneralReg(inst.dest, destV - srcV)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstSubReg8Reg8")
	}
	return state, nil
}

func execInstJb(inst instJb, state state) (state, error) {
	if state.isActiveCF() {
		state.ip = word(int16(state.ip) + int16(inst.rel8))
	}
	return state, nil
}

func execInstCld(inst instCld, state state) (state, error) {
	state = state.resetDF()
	return state, nil
}

func execScasb(state state, memory *memory) (state, error) {
	vAL, err := state.readByteGeneralReg(AL)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vSeg, err := state.readWordSreg(ES) // use ES for DI in string instructions
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vDI, err := state.readWordGeneralReg(DI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vMem, err := memory.readByte(state.realAddress(vSeg, vDI))
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	if vAL == vMem {
		state = state.setZF()
	} else {
		state = state.resetZF()
	}
	if state.isNotActiveDF() {
		state, err = state.writeWordGeneralReg(DI, vDI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	} else {
		state, err = state.writeWordGeneralReg(DI, vDI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	}
	return state, nil
}

func execMovsb(state state, memory *memory) (state, error) {
	vDS, err := state.readWordSreg(DS) // use DS for SI in string instructions
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vES, err := state.readWordSreg(ES) // use ES for DI in string instructions
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vSI, err := state.readWordGeneralReg(SI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vDI, err := state.readWordGeneralReg(DI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vMem, err := memory.readByte(state.realAddress(vDS, vSI))
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	err = memory.writeByte(state.realAddress(vES, vDI), vMem)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	if state.isNotActiveDF() {
		state, err = state.writeWordGeneralReg(SI, vSI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
		state, err = state.writeWordGeneralReg(DI, vDI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	} else {
		state, err = state.writeWordGeneralReg(SI, vSI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
		state, err = state.writeWordGeneralReg(DI, vDI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	}
	return state, nil
}

func execStosb(state state, memory *memory) (state, error) {
	vES, err := state.readWordSreg(ES)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	vDI, err := state.readWordGeneralReg(DI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	vAL, err := state.readByteGeneralReg(AL)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	err = memory.writeByte(state.realAddress(vES, vDI), vAL)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	if state.isNotActiveDF() {
		state, err = state.writeWordGeneralReg(DI, vDI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execStosb")
		}
	} else {
		state, err = state.writeWordGeneralReg(DI, vDI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execStosb")
		}
	}
	return state, nil
}

func execInstRepeScasb(inst instRepeScasb, state state, memory *memory) (state, error) {
	count, err := state.readWordGeneralReg(CX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	for count > 0 && state.isActiveZF() {
		state, err = execScasb(state, memory)
		if err != nil {
			return state, errors.Wrap(err, "failed in execInstRepeScasb")
		}
		count--
	}
	state, err = state.writeWordGeneralReg(CX, count)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	return state, nil
}

func execInstRepMovsb(inst instRepMovsb, state state, memory *memory) (state, error) {
	count, err := state.readWordGeneralReg(CX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	for count > 0 {
		state, err = execMovsb(state, memory)
		if err != nil {
			return state, errors.Wrap(err, "failed in execInstRepeScasb")
		}
		count--
	}
	state, err = state.writeWordGeneralReg(CX, count)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	return state, nil
}

func execInstRepStosb(inst instRepStosb, state state, memory *memory) (state, error) {
	count, err := state.readWordGeneralReg(CX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	for count > 0 {
		state, err = execStosb(state, memory)
		if err != nil {
			return state, errors.Wrap(err, "failed in execInstRepeScasb")
		}
		count--
	}
	state, err = state.writeWordGeneralReg(CX, count)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	return state, nil
}

func execInstStosb(inst instStosb, state state, memory *memory) (state, error) {
	return execStosb(state, memory)
}

func execInstJeRel8(inst instJeRel8, state state) (state, error) {
	if state.isActiveZF() {
		state.ip = word(int16(state.ip) + int16(inst.rel8))
	}
	return state, nil
}

func execInstInc(inst instInc, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	state, err = state.writeWordGeneralReg(inst.dest, v + 1)
	// TODO: Set ZF (so it is necessary to handle overflow...)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	return state, nil
}

func execInstDec(inst instDec, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	state, err = state.writeWordGeneralReg(inst.dest, v - 1)
	// TODO: Set ZF (so it is necessary to handle overflow...)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	return state, nil
}

func execInstXorReg16Reg16(inst instXorReg16Reg16, state state) (state, error) {
	destV, err := state.readWordGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstXorReg16Reg16")
	}
	srcV, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstXorReg16Reg16")
	}
	v := destV ^ srcV
	state, err = state.writeWordGeneralReg(inst.dest, v)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstXorReg16Reg16")
	}
	return state, nil
}

func execute(shouldBeInst interface{}, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	switch inst := shouldBeInst.(type) {
	case instMov:
		return execMov(inst, state)
	case instMovReg8Imm8:
		return execMovReg8Imm8(inst, state)
	case instMovRegReg:
		return execMovRegReg(inst, state)
	case instMovSRegReg:
		return execMovSRegReg(inst, state)
	case instMovRegMemBP:
		return execMovRegMemBP(inst, state, memory)
	case instMovReg8MemDisp8:
		return execMovReg8MemDisp8(inst, state, memory)
	case instShl:
		return execShl(inst, state)
	case instAdd:
		return execAdd(inst, state)
	case instSub:
		return execSub(inst, state)
	case instLea:
		return execLea(inst, state)
	case instLeaReg16Disp8:
		return execInstLeaReg16Disp8(inst, state, memory)
	case instInt:
		return execInt(inst, state, memory)
	case instPush:
		return execPush(inst, state, memory)
	case instPop:
		return execPop(inst, state, memory)
	case instCall:
		return execCall(inst, state, memory)
	case instRet:
		return execRet(inst, state, memory)
	case instJmpRel16:
		return execJmpRel16(inst, state, memory)
	case instSti:
		return execSti(inst, state, memory)
	case instAndReg8Imm8:
		return execAndReg8Imm8(inst, state, memory)
	case instAndMem8Reg8:
		return execAndMem8Reg8(inst, state, memory)
	case instMovMem16Reg16:
		return execMovMem16Reg16(inst, state, memory, segmentOverride)
	case instMovMem8Reg8:
		return execMovMem8Reg8(inst, state, memory, segmentOverride)
	case instMovReg16Mem16:
		return execMovReg16Mem16(inst, state, memory, segmentOverride)
	case instMovMem16Sreg:
		return execMovMem16Sreg(inst, state, memory, segmentOverride)
	case instAddReg16Reg16:
		return execAddReg16Reg16(inst, state)
	case instShrReg16Imm:
		return execShrReg16Imm(inst, state)
	case instShlReg16Imm:
		return execShlReg16Imm(inst, state)
	case instCmpMem8Imm8:
		return execInstCmpMem8Imm8(inst, state, memory, segmentOverride)
	case instCmpMem16Imm8:
		return execInstCmpMem16Imm8(inst, state, memory)
	case instCmpReg8Imm8:
		return execInstCmpReg8Imm8(inst, state, memory)
	case instCmpReg16Reg16:
		return execInstCmpReg16Reg16(inst, state)
	case instJneRel8:
		return execInstJneRel8(inst, state)
	case instMovReg16Sreg:
		return execInstMovReg16Sreg(inst, state)
	case instSubReg16Reg16:
		return execInstSubReg16Reg16(inst, state)
	case instSubReg8Reg8:
		return execInstSubReg8Reg8(inst, state)
	case instJb:
		return execInstJb(inst, state)
	case instCld:
		return execInstCld(inst, state)
	case instRepeScasb:
		return execInstRepeScasb(inst, state, memory)
	case instRepMovsb:
		return execInstRepMovsb(inst, state, memory)
	case instRepStosb:
		return execInstRepStosb(inst, state, memory)
	case instJeRel8:
		return execInstJeRel8(inst, state)
	case instInc:
		return execInstInc(inst, state)
	case instStosb:
		return execInstStosb(inst, state, memory)
	case instDec:
		return execInstDec(inst, state)
	case instXorReg16Reg16:
		return execInstXorReg16Reg16(inst, state)
	default:
		return state, errors.Errorf("unknown inst: %T", shouldBeInst)
	}
}

func runExeWithCustomIntHandlers(reader io.Reader, intHandlers intHandlers) (state, error) {
	parser := newParser(reader)
	header, loadModule, err := parseHeaderWithParser(parser)
	if err != nil {
		return state{}, errors.Wrap(err, "error to parse header")
	}

	memory := newMemoryFromHeader(loadModule, header)

	s := newState(header, intHandlers)

	for {
		inst, readBytesCount, segmentOverride, err := decodeInstWithMemory(s.realIP(), memory)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			} else {
				return state{}, errors.Wrap(err, "error to decode inst")
			}
		}
		debug.printf("decode inst %#v at 0x%04x:0x%04x\n", inst, s.cs, s.ip)

		s.ip = s.ip + word(readBytesCount)
		s, err = execute(inst, s, memory, segmentOverride)
		if err != nil {
			return state{}, errors.Wrap(err, "errors to execute")
		}
		if s.shouldExit {
			break
		}
		// x, _ := s.readWordGeneralReg(AX)
		// debug.printf("0x%04x\n", x)
		// v, _ = s.readWordGeneralReg(CX)
		// debug.printf("0x%04x\n", v)
		// z, _ := memory.readWord(s.realAddress(s.ds, 0x0042))
		// debug.printf("0x%04x\n", z)
	}

	return s, nil
}

// (exit code, state, error)
func RunExe(reader io.Reader) (uint8, state, error) {
	state, err := runExeWithCustomIntHandlers(reader, make(intHandlers))
	return uint8(state.exitCode), state, err
}
