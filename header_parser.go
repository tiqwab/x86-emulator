package x86_emulator

import (
	"bufio"
	"fmt"
	"github.com/pkg/errors"
	"io"
)

const (
	paragraphSize int = 16
)

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

