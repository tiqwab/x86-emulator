package x86_emulator

import (
	"io"
	"bufio"
	"fmt"
	"os"
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
		return nil, err
	}
	exSignature := [2]byte{buf[0], buf[1]}

	_, err = parseBytes(6, sc)
	if err != nil {
		return nil, err
	}

	exHeaderSize, err := parseWord(sc)
	if err != nil {
		return nil, err
	}

	_, err = parseBytes(4, sc)
	if err != nil {
		return nil, err
	}

	exInitSS, err := parseWord(sc)
	if err != nil {
		return nil, err
	}

	exInitSP, err := parseWord(sc)
	if err != nil {
		return nil, err
	}

	_, err = parseBytes(2, sc)
	if err != nil {
		return nil, err
	}

	exInitIP, err := parseWord(sc)
	if err != nil {
		return nil, err
	}

	exInitCS, err := parseWord(sc)
	if err != nil {
		return nil, err
	}

	_, err = parseBytes(8, sc)
	if err != nil {
		return nil, err
	}

	return &header{
		exSignature: exSignature,
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
				return nil, err
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
		return 0, err
	}
	return bs[0], nil
}

// assume little-endian
func parseWord(sc *bufio.Scanner) (word, error) {
	buf, err := parseBytes(2, sc)
	if err != nil {
		return 0, err
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

type instInt struct {
	operand uint8
}

type instMov struct {
	dest registerW
	imm word
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
		return 0, 0, 0, err
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
		return inst, err
	}

	switch rawOpcode {
	// add r/m16, imm8
	case 0x83:
		mod, reg, rm, err := decodeModRegRM(sc)
		if err != nil {
			return inst, err
		}

		if mod != 3 {
			return nil, fmt.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 0 {
			return nil, fmt.Errorf("expect reg is /0 but %d", reg)
		}

		imm, err := parseByte(sc)

		switch rm {
		case AX:
			inst = instAdd{dest: AX, imm: imm}
		case CX:
			inst = instAdd{dest: CX, imm: imm}
		default:
			return nil, fmt.Errorf("unknown register: %d", rm)
		}

	// mov r16,imm16
	case 0xb8:
		// ax
		imm, err := parseWord(sc)
		if err != nil {
			return inst, err
		}
		inst = instMov{dest: AX, imm: imm}
	case 0xb9:
		// cx
		imm, err := parseWord(sc)
		if err != nil {
			return inst, err
		}
		inst = instMov{dest: CX, imm: imm}

	// shl r/m16,imm8
	// FIXME: handle memory address as source
	case 0xc1:
		mod, reg, rm, err := decodeModRegRM(sc)
		if err != nil {
			return inst, err
		}

		if mod != 3 {
			return nil, fmt.Errorf("expect mod is 0b11 but %02b", mod)
		}
		if reg != 4 {
			return nil, fmt.Errorf("expect reg is /4 but %d", reg)
		}

		imm, err := parseByte(sc)

		switch rm {
		case AX:
			inst = instShl{register: AX, imm: imm}
		case CX:
			inst = instShl{register: CX, imm: imm}
		default:
			return nil, fmt.Errorf("unknown register: %d", rm)
		}

	// int imm8
	case 0xcd:
		operand, err := parseByte(sc)
		if err != nil {
			return inst, err
		}
		inst = instInt{operand: operand}
	default:
		return inst, fmt.Errorf("unknown opcode: 0x%02x", rawOpcode)
	}
	return inst, nil
}

type state struct {
	ax, cx, ss, sp, cs, ip word
}

func newState(ss, sp, cs, ip word) state {
	return state{}
}

func execMov(inst instMov, state state) (state, error) {
	switch inst.dest {
	case AX:
		state.ax = inst.imm
	case CX:
		state.cx = inst.imm
	default:
		return state, fmt.Errorf("unknown register: %v", inst.dest)
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
		return state, fmt.Errorf("unknown register: %v", inst.register)
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
		return state, fmt.Errorf("unknown register: %v", inst.dest)
	}
	return state, nil
}

func execInt(inst instInt, state state) (state, error) {
	switch inst.operand {
	case 21:
		os.Exit(99) // FIXME: accept exitcode
	default:
		return state, fmt.Errorf("unknown operand: %v", inst.operand)
	}
	return state, nil
}

func execute(shouldBeInst interface{}, state state) (state, error) {
	switch inst := shouldBeInst.(type) {
	case instMov:
		return execMov(inst, state)
	case instShl:
		return execShl(inst, state)
	case instAdd:
		return execAdd(inst, state)
	case instInt:
		return execInt(inst, state)
	default:
		return state, fmt.Errorf("unknown inst: %T", shouldBeInst)
	}
}

func RunExe(reader io.Reader) (state, error) {
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)
	header, err := parseHeaderWithScanner(sc)
	if err != nil {
		return state{}, err
	}

	s := newState(header.exInitSS, header.exInitSP, header.exInitCS, header.exInitIP)

	for {
		inst, err := decodeInstWithScanner(sc)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return state{}, err
			}
		}

		s, err = execute(inst, s)
		if err != nil {
			return state{}, err
		}
	}

	return s, nil
}
