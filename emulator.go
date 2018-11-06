package x86_emulator

import (
	"io"
	"bufio"
	"fmt"
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

func ParseHeader(reader io.Reader) (*header, error) {
	var buf []byte
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)

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
			return nil, fmt.Errorf("failed to parse %d bytes\n", n)
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

type instInt struct{
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

func DecodeInst(reader io.Reader) (interface{}, error) {
	var inst interface{}
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)

	rawOpcode, err := parseByte(sc)
	if err != nil {
		return inst, err
	}

	switch rawOpcode {
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
		buf, err := parseByte(sc)
		if err != nil {
			return inst, err
		}

		mod := (buf & 0xc0) >> 6     // 0b11000000
		reg := (buf & 0x38) >> 3     // 0b00111000
		rm  := registerW(buf & 0x07) // 0b00000111

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
		return inst, fmt.Errorf("unknown opcode: %v", rawOpcode)
	}
	return inst, nil
}
