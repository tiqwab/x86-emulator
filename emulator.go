package x86_emulator

import (
	"io"
	"bufio"
	"fmt"
	"os"
	"github.com/pkg/errors"
)

// ref1. https://en.wikibooks.org/wiki/X86_Assembly/Machine_Language_Conversion
// ref2. https://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-instruction-set-reference-manual-325383.pdf

type word uint16

type exe struct {
	rawHeader []byte
	header header
}

type header struct {
	exSignature [2]byte
	relocationItems word
	exHeaderSize word
	exInitSS word
	exInitSP word
	exInitIP word
	exInitCS word
}

func (h header) String() string {
	return fmt.Sprintf("header{exSignature: %v, exHeaderSize: %d, exInitSS: 0x%04X, exInitSP: 0x%04X, exInitIP: 0x%04X, exInitCS: 0x%04X}",
		h.exSignature, h.exHeaderSize, h.exInitSS, h.exInitSP, h.exInitIP, h.exInitCS)
}

func parseHeader(reader io.Reader) (*header, error) {
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)
	return parseHeaderWithScanner(sc)
}

func parseHeaderWithScanner(sc *bufio.Scanner) (*header, error) {
	var buf []byte

	buf, err := parseBytes(2, sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 0-1 of header")
	}
	exSignature := [2]byte{buf[0], buf[1]}

	_, err = parseBytes(4, sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 2-5 of header")
	}

	relocationItems, err := parseWord(sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 6-7 of header")
	}

	exHeaderSize, err := parseWord(sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 8-9 of header")
	}

	_, err = parseBytes(4, sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 10-13 of header")
	}

	exInitSS, err := parseWord(sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 14-15 of header")
	}

	exInitSP, err := parseWord(sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 16-17 of header")
	}

	_, err = parseBytes(2, sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 18-19 of header")
	}

	exInitIP, err := parseWord(sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 20-21 of header")
	}

	exInitCS, err := parseWord(sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bytes at 22-23 of header")
	}

	_, err = parseBytes(8, sc)
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
	}, nil
}

func parseBytes(n int, sc *bufio.Scanner) ([]byte, error) {
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		if b := sc.Scan(); b {
			buf[i] = sc.Bytes()[0]
		} else {
			if err := sc.Err(); err != nil {
				return nil, errors.Wrap(err, "failed to parse bytes")
			} else {
				return nil, io.EOF
			}
		}
	}
	return buf, nil
}

func parseByte(sc *bufio.Scanner) (byte, error) {
	bs, err := parseBytes(1, sc)
	if err != nil {
		if err := sc.Err(); err != nil {
			return 0, errors.Wrap(err, "failed to parse byte")
		} else {
			return 0, io.EOF
		}
	}
	return bs[0], nil
}

// assume little-endian
func parseWord(sc *bufio.Scanner) (word, error) {
	buf, err := parseBytes(2, sc)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse word")
	}
	return word(buf[1]) << 8 + word(buf[0]), nil
}

type registerW uint8

// ref2. 3.1.1.1
const (
	AX = registerW(0)
	CX = registerW(1)
	DX = registerW(2)
	BX = registerW(3)
	SP = registerW(4)
	BP = registerW(5)
	SI = registerW(6)
	DI = registerW(7)
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
	}
	return 0, errors.Errorf("illegal number for registerW: %d", x)
}

type instInt struct {
	operand uint8
}

type instMov struct {
	dest registerW
	imm word
}

type instMovRegReg struct {
	dest registerW
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

func decodeModRegRM(sc *bufio.Scanner) (byte, byte, registerW, error) {
	buf, err := parseByte(sc)
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed to parse byte")
	}

	mod := (buf & 0xc0) >> 6     // 0b11000000
	reg := (buf & 0x38) >> 3     // 0b00111000
	rm  := registerW(buf & 0x07) // 0b00000111

	return mod, reg, rm, nil
}

func decodeInst(reader io.Reader) (interface{}, error) {
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)
	return decodeInstWithScanner(sc)
}

func decodeInstWithScanner(sc *bufio.Scanner) (interface{}, error) {
	var inst interface{}

	rawOpcode, err := parseByte(sc)
	if err != nil {
		return inst, errors.Wrap(err, "failed to parse opcode")
	}

	switch rawOpcode {
	// add r/m16, imm8
	case 0x83:
		mod, reg, rm, err := decodeModRegRM(sc)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod != 3 {
			return nil, errors.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 0 {
			return nil, errors.Errorf("expect reg is /0 but %d", reg)
		}

		imm, err := parseByte(sc)
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
		mod, reg, rm, err := decodeModRegRM(sc)
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

	// mov r16,imm16
	case 0xb8:
		// ax
		imm, err := parseWord(sc)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}
		inst = instMov{dest: AX, imm: imm}
	case 0xb9:
		// cx
		imm, err := parseWord(sc)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}
		inst = instMov{dest: CX, imm: imm}

	// shl r/m16,imm8
	// FIXME: handle memory address as source
	case 0xc1:
		mod, reg, rm, err := decodeModRegRM(sc)
		if err != nil {
			return inst, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		if mod != 3 {
			return nil, errors.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 4 {
			return nil, errors.Errorf("expect reg is /4 but %d", reg)
		}

		imm, err := parseByte(sc)
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
		operand, err := parseByte(sc)
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
type intHandler func(state) error
type intHandlers map[uint8]intHandler

type state struct {
	ax, cx, ss, sp, cs, ip word
	intHandlers intHandlers
}

func newState(ss, sp, cs, ip word, customIntHandlers intHandlers) state {
	intHandlers := make(intHandlers)
	for k, v := range customIntHandlers {
		intHandlers[k] = v
	}

	// int 21 4c
	if _, ok := intHandlers[0x4c]; !ok {
		intHandlers[0x4c] = func(s state) error {
			os.Exit(int(s.al()))
			return nil
		}
	}

	return state{intHandlers: intHandlers}
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

func execInt(inst instInt, state state) (state, error) {
	switch inst.operand {
	case 0x21:
		if handler, ok := state.intHandlers[state.ah()]; ok {
			err := handler(state)
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
	case instMovRegReg:
		return execMovRegReg(inst, state)
	case instShl:
		return execShl(inst, state)
	case instAdd:
		return execAdd(inst, state)
	case instInt:
		return execInt(inst, state)
	default:
		return state, errors.Errorf("unknown inst: %T", shouldBeInst)
	}
}

func runExeWithCustomIntHandlers(reader io.Reader, intHandlers intHandlers) (state, error) {
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)
	header, err := parseHeaderWithScanner(sc)
	if err != nil {
		return state{}, errors.Wrap(err, "error to parse header")
	}

	s := newState(header.exInitSS, header.exInitSP, header.exInitCS, header.exInitIP, intHandlers)

	for {
		inst, err := decodeInstWithScanner(sc)
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
	}

	return s, nil
}

func RunExe(reader io.Reader) (state, error) {
	return runExeWithCustomIntHandlers(reader, make(intHandlers))
}
