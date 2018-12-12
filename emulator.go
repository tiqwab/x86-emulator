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
type exitCode uint8

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

type instMovB struct {
	dest registerB
	imm uint8
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
func decodeInst(reader io.Reader) (interface{}, int, error) {
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, 0, err
	}
	memory := newMemory(bytes)
	return decodeInstWithMemory(0, memory)
}

// inst, read bytes, error
func decodeInstWithMemory(initialAddress address, memory *memory) (interface{}, int, error) {
	var inst interface{}
	currentAddress := initialAddress

	rawOpcode, err := memory.readByte(currentAddress)
	currentAddress++
	if err != nil {
		return inst, -1, errors.Wrap(err, "failed to parse opcode")
	}

	switch rawOpcode {
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

	// add r/m16, imm8
	// 83 /5 -> sub r/m16, imm8
	case 0x83:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod != 3 {
			return nil, -1, errors.Errorf("expect mod is 0b11 but %02b", mod)
		}
		switch reg {
		// add
		case 0:
			imm, err := memory.readByte(currentAddress)
			currentAddress++
			if err != nil {
				return inst, -1, errors.Wrap(err, "failed to parse imm")
			}

			switch rm {
			case AX:
				inst = instAdd{dest: AX, imm: imm}
			case DX:
				inst = instAdd{dest: DX, imm: imm}
			case CX:
				inst = instAdd{dest: CX, imm: imm}
			case SP:
				inst = instAdd{dest: SP, imm: imm}
			default:
				return nil, -1, errors.Errorf("unknown register: %d", rm)
			}
		// sub
		case 5:
			imm, err := memory.readByte(currentAddress)
			currentAddress++
			if err != nil {
				return inst, -1, errors.Wrap(err, "failed to decode sub inst")
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
				return nil, -1, errors.Errorf("unknown register: %d", rm)
			}
		default:
			return nil, -1, errors.Errorf("expect reg is 0 but %d", reg)
		}

	// 8b /r (/r indicates that the ModR/M byte of the instruction contains a register operand and an r/m operand)
	// mov r16,r/m16
	case 0x8b:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			dest, err := toRegisterW(uint8(reg))
			if err != nil {
				return inst, -1, errors.Errorf("illegal reg value for registerW: %d", reg)
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, errors.Errorf("illegal rm value for registerW: %d", rm)
			}
			inst = instMovRegReg{dest: dest, src: src}
		} else if mod == 1 {
			dest, err := toRegisterW(uint8(reg))
			if err != nil {
				return inst, -1, errors.Errorf("illegal reg value for registerW: %d", reg)
			}
			switch(rm) {
			case 6:
				disp, err := memory.readInt8(currentAddress)
				currentAddress++
				if err != nil {
					return inst, -1, errors.Errorf("failed to read as int8")
				}
				inst = instMovRegMemBP{dest: dest, disp: disp}
			default:
				return inst, -1, errors.Errorf("not yet implemented for rm %d", rm)
			}
		} else {
			return inst, -1, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// 8d /r
	// lea r16,m
	case 0x8d:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 0 && rm == 6 {
			dest, err := toRegisterW(reg)
			if err != nil {
				return inst, -1, errors.Wrap(err, "illegal reg value for registerW")
			}
			address, err := memory.readWord(currentAddress)
			currentAddress += 2
			if err != nil {
				return inst, -1, errors.Wrap(err, "failed to parse address of lea")
			}
			inst = instLea{dest: dest, address: address}
		} else {
			return inst, -1, errors.Errorf("mod and rm should be 0 and 3 respectively for lea? mod=%d, rm=%d actually", mod, rm)
		}

	// 8e /r
	// mov Sreg,r/m16
	// Sreg ES=0, CS=1, SS=2, DS=3, FS=4, GS=5
	case 0x8e:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			dest, err := toRegisterS(reg)
			if err != nil {
				return inst, -1, errors.Errorf("illegal reg value for registerS: %d", reg)
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, -1, errors.Errorf("illegal reg value for registerW: %d", rm)
			}
			inst = instMovSRegReg{dest: dest, src: src}
		} else {
			return inst, -1, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// b0+ rb ib
	// mov r8,imm8
	case 0xb4:
		// ah
		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode imm")
		}
		inst = instMovB{dest: AH, imm: imm}

	// b8+ rw iw
	// mov r16,imm16
	case 0xb8:
		// ax
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode imm")
		}
		inst = instMov{dest: AX, imm: imm}
	case 0xb9:
		// cx
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode imm")
		}
		inst = instMov{dest: CX, imm: imm}
	case 0xba:
		// dx
		imm, err := memory.readWord(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode imm")
		}
		inst = instMov{dest: DX, imm: imm}

	// shl r/m16,imm8
	// FIXME: handle memory address as source
	case 0xc1:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		currentAddress++
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod != 3 {
			return nil, -1, errors.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 4 {
			return nil, -1, errors.Errorf("expect reg is /4 but %d", reg)
		}

		imm, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			errors.Wrap(err, "failed to parse imm")
		}

		switch rm {
		case AX:
			inst = instShl{register: AX, imm: imm}
		case CX:
			inst = instShl{register: CX, imm: imm}
		default:
			return nil, -1, errors.Errorf("unknown register: %d", rm)
		}

	// ret (near return)
	case 0xc3:
		inst = instRet{}

	// int imm8
	case 0xcd:
		operand, err := memory.readByte(currentAddress)
		currentAddress++
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to parse operand")
		}
		inst = instInt{operand: operand}

	// call rel16
	case 0xe8:
		rel, err := memory.readInt16(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to parse int16")
		}
		inst = instCall{rel: rel}

	// jmp rel16
	case 0xe9:
		rel, err := memory.readInt16(currentAddress)
		currentAddress += 2
		if err != nil {
			return inst, -1, errors.Wrap(err, "failed to parse int16")
		}
		inst = instJmpRel16{rel: rel}

	// sti
	case 0xfb:
		inst = instSti{}

	default:
		return inst, -1, errors.Errorf("unknown opcode: 0x%02x", rawOpcode)
	}
	return inst, int(currentAddress - initialAddress), nil
}

// for int 21
type intHandler func(*state, *memory) error
type intHandlers map[uint8]intHandler

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
	ax, cx, dx, bx, sp, bp, si, di, ss, cs, ip, ds word
	exitCode                   exitCode
	shouldExit                 bool
	intHandlers                intHandlers
}

func newState(header *header, customIntHandlers intHandlers) state {
	// --- Prepare interrupted handlers

	intHandlers := make(intHandlers)
	for k, v := range customIntHandlers {
		intHandlers[k] = v
	}

	// int 21 4ch
	if _, ok := intHandlers[0x4c]; !ok {
		intHandlers[0x4c] = func(s *state, m *memory) error {
			return intHandler4c(s, m)
		}
	}

	// int 21 09h
	if _, ok := intHandlers[0x09]; !ok {
		intHandlers[0x09] = func(s *state, m *memory) error {
			return intHandler09(s, m)
		}
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

func (s state) ah() uint8 {
	return uint8(s.ax >> 8)
}

func (s state) realAddress(sreg word, reg word) address {
	return realAddress(sreg, reg)
}

func (s state) realIP() address {
	return address(s.cs << 4 + s.ip)
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
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execMovB(inst instMovB, state state) (state, error) {
	switch inst.dest {
	case AH:
		state.ax = word((uint16(inst.imm) << 8) + (0x0011) & uint16(state.ax))
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
	case SP:
		state.sp = v
	case BP:
		state.bp = v
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execMovSRegReg(inst instMovSRegReg, state state) (state, error) {
	switch inst.dest {
	case DS:
		v, err := state.readWordGeneralReg(inst.src)
		if err != nil {
			return state, errors.Wrap(err, "failed to get reg")
		}
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

func execShl(inst instShl, state state) (state, error) {
	switch inst.register {
	case AX:
		state.ax <<= inst.imm
	case CX:
		state.cx <<= inst.imm
	default:
		return state, errors.Errorf("unknown register: %v", inst.register)
	}
	return state, nil
}

func execAdd(inst instAdd, state state) (state, error) {
	switch inst.dest {
	case AX:
		state.ax += word(inst.imm)
	case DX:
		state.dx += word(inst.imm)
	case CX:
		state.cx += word(inst.imm)
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

func execute(shouldBeInst interface{}, state state, memory *memory) (state, error) {
	switch inst := shouldBeInst.(type) {
	case instMov:
		return execMov(inst, state)
	case instMovB:
		return execMovB(inst, state)
	case instMovRegReg:
		return execMovRegReg(inst, state)
	case instMovSRegReg:
		return execMovSRegReg(inst, state)
	case instMovRegMemBP:
		return execMovRegMemBP(inst, state, memory)
	case instShl:
		return execShl(inst, state)
	case instAdd:
		return execAdd(inst, state)
	case instSub:
		return execSub(inst, state)
	case instLea:
		return execLea(inst, state)
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
		inst, readBytesCount, err := decodeInstWithMemory(s.realIP(), memory)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			} else {
				return state{}, errors.Wrap(err, "error to decode inst")
			}
		}
		debug.printf("decode inst %#v at 0x%04x:0x%04x\n", inst, s.cs, s.ip)

		s.ip = s.ip + word(readBytesCount)
		s, err = execute(inst, s, memory)
		if err != nil {
			return state{}, errors.Wrap(err, "errors to execute")
		}
		if s.shouldExit {
			break
		}
	}

	return s, nil
}

// (exit code, state, error)
func RunExe(reader io.Reader) (uint8, state, error) {
	state, err := runExeWithCustomIntHandlers(reader, make(intHandlers))
	return uint8(state.exitCode), state, err
}
