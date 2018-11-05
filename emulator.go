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

func ParseHeader(reader io.Reader) *header {
	buf := make([]byte, 256)
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanBytes)

	buf = parseBytes(2, sc)
	exSignature := [2]byte{buf[0], buf[1]}

	parseBytes(6, sc)

	exHeaderSize := parseWord(sc)

	parseBytes(4, sc)

	exInitSS := parseWord(sc)
	exInitSP := parseWord(sc)

	parseBytes(2, sc)

	exInitIP := parseWord(sc)
	exInitCS := parseWord(sc)

	return &header{
		exSignature: exSignature,
		exHeaderSize: exHeaderSize,
		exInitSS: exInitSS,
		exInitSP: exInitSP,
		exInitIP: exInitIP,
		exInitCS: exInitCS,
	}
}

func parseBytes(n int, sc *bufio.Scanner) []byte {
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		if b := sc.Scan(); b {
			buf[i] = sc.Bytes()[0]
		} else {
			return nil
		}
	}
	return buf
}

// assume little-endian
func parseWord(sc *bufio.Scanner) word {
	buf := parseBytes(2, sc)
	return word(buf[1]) << 8 + word(buf[0])
}
