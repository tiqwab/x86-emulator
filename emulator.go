package x86_emulator

import (
	"io"
	"bufio"
	"fmt"
)

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

type instInt struct{
	operand uint8
}

func DecodeInst(reader io.Reader) (instInt, error) {
	var inst instInt
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)

	rawOpcode, err := parseByte(sc)
	if err != nil {
		return inst, err
	}

	switch rawOpcode {
	case 0xcd:
		operand, err := parseByte(sc)
		if err != nil {
			return inst, err
		}
		inst = instInt{operand: operand}
	default:
		return inst, fmt.Errorf("unknown opcode: %v\n", rawOpcode)
	}
	return inst, nil
}
