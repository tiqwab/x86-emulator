package x86_emulator

import (
	"io"
	"bufio"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
)

// ref1. https://en.wikibooks.org/wiki/X86_Assembly/Machine_Language_Conversion
// ref2. https://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-instruction-set-reference-manual-325383.pdf

const (
	paragraphSize int = 16
)

type word uint16
type exitCode uint8

type exe struct {
	rawHeader []byte
	header header
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

func parseHeader(reader io.Reader) (*header, error) {
	parser := newParser(reader)
	return parseHeaderWithParser(parser)
}

func parseHeaderWithParser(parser *parser) (*header, error) {
	var buf []byte

	buf, err := parser.parseBytes(2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 0-1 of header")
	}
	exSignature := [2]byte{buf[0], buf[1]}

	_, err = parser.parseBytes(4)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 2-5 of header")
	}

	relocationItems, err := parser.parseWord()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 6-7 of header")
	}

	exHeaderSize, err := parser.parseWord()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 8-9 of header")
	}

	_, err = parser.parseBytes(4)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 10-13 of header")
	}

	exInitSS, err := parser.parseWord()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 14-15 of header")
	}

	exInitSP, err := parser.parseWord()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 16-17 of header")
	}

	_, err = parser.parseBytes(2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 18-19 of header")
	}

	exInitIP, err := parser.parseWord()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 20-21 of header")
	}

	exInitCS, err := parser.parseWord()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 22-23 of header")
	}

	relocationTableOffset, err := parser.parseWord()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 24-25 of header")
	}

	remainHeaderBytes := int(exHeaderSize) * paragraphSize - int(parser.offset)
	_, err = parser.parseBytes(remainHeaderBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 24-31 of header")
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
	}, nil
}

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

type instShl struct {
	register registerW
	imm uint8
}

type instAdd struct {
	dest registerW
	imm uint8
}

type instLea struct {
	dest registerW
	address word
}

func decodeModRegRM(parser *parser) (byte, byte, registerW, error) {
	buf, err := parser.parseByte()
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed to parse byte")
	}

	mod := (buf & 0xc0) >> 6     // 0b11000000
	reg := (buf & 0x38) >> 3     // 0b00111000
	rm  := registerW(buf & 0x07) // 0b00000111

	return mod, reg, rm, nil
}

func decodeInst(reader io.Reader) (interface{}, error) {
	parser := newParser(reader)
	return decodeInstWithParser(parser)
}

func decodeInstWithParser(parser *parser) (interface{}, error) {
	var inst interface{}

	rawOpcode, err := parser.parseByte()
	if err != nil {
		return inst, errors.Wrap(err, "failed to parse opcode")
	}

	switch rawOpcode {
	// add r/m16, imm8
	case 0x83:
		mod, reg, rm, err := decodeModRegRM(parser)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod != 3 {
			return nil, errors.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 0 {
			return nil, errors.Errorf("expect reg is /0 but %d", reg)
		}

		imm, err := parser.parseByte()
		if err != nil {
			return inst, errors.Wrap(err, "failed to parse imm")
		}

		switch rm {
		case AX:
			inst = instAdd{dest: AX, imm: imm}
		case CX:
			inst = instAdd{dest: CX, imm: imm}
		default:
			return nil, errors.Errorf("unknown register: %d", rm)
		}

	// 8b /r (/r indicates that the ModR/M byte of the instruction contains a register operand and an r/m operand)
	// mov r16,r/m16
	case 0x8b:
		mod, reg, rm, err := decodeModRegRM(parser)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			dest, err := toRegisterW(uint8(reg))
			if err != nil {
				return inst, errors.Errorf("illegal reg value for registerW: %d", reg)
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, errors.Errorf("illegal rm value for registerW: %d", rm)
			}
			inst = instMovRegReg{dest: dest, src: src}
		} else {
			return inst, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// 8d /r
	// lea r16,m
	case 0x8d:
		mod, reg, rm, err := decodeModRegRM(parser)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 0 && rm == 6 {
			dest, err := toRegisterW(reg)
			if err != nil {
				return inst, errors.Wrap(err, "illegal reg value for registerW")
			}
			address, err := parser.parseWord()
			if err != nil {
				return inst, errors.Wrap(err, "failed to parse address of lea")
			}
			inst = instLea{dest: dest, address: address}
		} else {
			return inst, errors.Errorf("mod and rm should be 0 and 3 respectively for lea? mod=%d, rm=%d actually", mod, rm)
		}

	// 8e /r
	// mov Sreg,r/m16
	// Sreg ES=0, CS=1, SS=2, DS=3, FS=4, GS=5
	case 0x8e:
		mod, reg, rm, err := decodeModRegRM(parser)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod == 3 {
			dest, err := toRegisterS(reg)
			if err != nil {
				return inst, errors.Errorf("illegal reg value for registerS: %d", reg)
			}
			src, err := toRegisterW(uint8(rm))
			if err != nil {
				return inst, errors.Errorf("illegal reg value for registerW: %d", rm)
			}
			inst = instMovSRegReg{dest: dest, src: src}
		} else {
			return inst, errors.Errorf("not yet implemented for mod 0x%02x", mod)
		}

	// b0+ rb ib
	// mov r8,imm8
	case 0xb4:
		// ah
		imm, err := parser.parseByte()
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode imm")
		}
		inst = instMovB{dest: AH, imm: imm}

	// b8+ rw iw
	// mov r16,imm16
	case 0xb8:
		// ax
		imm, err := parser.parseWord()
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode imm")
		}
		inst = instMov{dest: AX, imm: imm}
	case 0xb9:
		// cx
		imm, err := parser.parseWord()
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode imm")
		}
		inst = instMov{dest: CX, imm: imm}

	// shl r/m16,imm8
	// FIXME: handle memory address as source
	case 0xc1:
		mod, reg, rm, err := decodeModRegRM(parser)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod != 3 {
			return nil, errors.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 4 {
			return nil, errors.Errorf("expect reg is /4 but %d", reg)
		}

		imm, err := parser.parseByte()
		if err != nil {
			errors.Wrap(err, "failed to parse imm")
		}

		switch rm {
		case AX:
			inst = instShl{register: AX, imm: imm}
		case CX:
			inst = instShl{register: CX, imm: imm}
		default:
			return nil, errors.Errorf("unknown register: %d", rm)
		}

	// int imm8
	case 0xcd:
		operand, err := parser.parseByte()
		if err != nil {
			return inst, errors.Wrap(err, "failed to parse operand")
		}
		inst = instInt{operand: operand}
	default:
		return inst, errors.Errorf("unknown opcode: 0x%02x", rawOpcode)
	}
	return inst, nil
}

// for int 21
type intHandler func(*state) error
type intHandlers map[uint8]intHandler

func intHandler4c(s *state) error {
	s.exitCode = exitCode(s.al())
	s.shouldExit = true
	return nil
}

func intHandler09(s *state) error {
	// TODO: implement
	fmt.Println("should be implement print")
	return nil
}

type memory []byte

type state struct {
	ax, cx, ss, sp, cs, ip, ds word
	memory memory
	exitCode                   exitCode
	shouldExit                 bool
	intHandlers                intHandlers
}

func newState(header *header, loadModule []byte, customIntHandlers intHandlers) state {
	// --- Prepare interrupted handlers

	intHandlers := make(intHandlers)
	for k, v := range customIntHandlers {
		intHandlers[k] = v
	}

	// int 21 4ch
	if _, ok := intHandlers[0x4c]; !ok {
		intHandlers[0x4c] = func(s *state) error {
			return intHandler4c(s)
		}
	}

	// int 21 09h
	if _, ok := intHandlers[0x09]; !ok {
		intHandlers[0x09] = func(s *state) error {
			return intHandler09(s)
		}
	}

	// --- Prepare memory (assuming that cs=0)
	loadModuleSize := len(loadModule)
	memory := make(memory, loadModuleSize)
	for i := 0; i < loadModuleSize; i++ {
		memory[i] = loadModule[i]
	}

	return state{memory: memory, intHandlers: intHandlers}
}

func (s state) al() uint8 {
	return uint8(s.ax & 0x00ff)
}

func (s state) ah() uint8 {
	return uint8(s.ax >> 8)
}

func (s state) reg(r registerW) (word, error) {
	switch r {
	case AX:
		return s.ax, nil
	case CX:
		return s.cx, nil
	default:
		return 0, errors.Errorf("illegal registerW or not implemented: %d", r)
	}
}

func execMov(inst instMov, state state) (state, error) {
	switch inst.dest {
	case AX:
		state.ax = inst.imm
	case CX:
		state.cx = inst.imm
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
	switch inst.dest {
	case AX:
		v, err := state.reg(inst.src)
		if err != nil {
			return state, errors.Wrap(err, "failed to get reg")
		}
		state.ax = v
	case CX:
		v, err := state.reg(inst.src)
		if err != nil {
			return state, errors.Wrap(err, "failed to get reg")
		}
		state.cx = v
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execMovSRegReg(inst instMovSRegReg, state state) (state, error) {
	switch inst.dest {
	case DS:
		v, err := state.reg(inst.src)
		if err != nil {
			return state, errors.Wrap(err, "failed to get reg")
		}
		state.ds = v
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
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
	case CX:
		state.cx += word(inst.imm)
	default:
		return state, errors.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execLea(inst instLea, state state) (state, error) {
	// TODO: implement
	return state, nil
}

func execInt(inst instInt, state state) (state, error) {
	switch inst.operand {
	case 0x21:
		if handler, ok := state.intHandlers[state.ah()]; ok {
			err := handler(&state)
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

func execute(shouldBeInst interface{}, state state) (state, error) {
	switch inst := shouldBeInst.(type) {
	case instMov:
		return execMov(inst, state)
	case instMovB:
		return execMovB(inst, state)
	case instMovRegReg:
		return execMovRegReg(inst, state)
	case instMovSRegReg:
		return execMovSRegReg(inst, state)
	case instShl:
		return execShl(inst, state)
	case instAdd:
		return execAdd(inst, state)
	case instLea:
		return execLea(inst, state)
	case instInt:
		return execInt(inst, state)
	default:
		return state, errors.Errorf("unknown inst: %T", shouldBeInst)
	}
}

func runExeWithCustomIntHandlers(reader io.Reader, intHandlers intHandlers) (state, error) {
	parser := newParser(reader)
	header, err := parseHeaderWithParser(parser)
	if err != nil {
		return state{}, errors.Wrap(err, "error to parse header")
	}

	loadModule, err := ioutil.ReadAll(reader)
	if err != nil {
		return state{}, errors.Wrap(err, "error to parse load module")
	}

	s := newState(header, loadModule, intHandlers)

	for {
		inst, err := decodeInstWithParser(parser)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			} else {
				return state{}, errors.Wrap(err, "error to decode inst")
			}
		}

		s, err = execute(inst, s)
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
