package x86_emulator

import (
	"testing"
	"bytes"
	"io"
	)

type machineCode []byte

func rawHeader() machineCode {
	// 32 bytes
	return []byte{
		0x4d, 0x5a, 0x2b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x01, 0xff, 0xff, 0x01, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestExample1(t *testing.T) {
	t.Log("example1")
}

func TestParseHeaderSignature(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, err := parseHeader(reader)
	if err != nil {
		t.Error(err)
	}
	expected := [2]byte{'M', 'Z'}
	if actual.exSignature != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exSignature)
	}
}

func TestParseHeaderSize(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, err := parseHeader(reader)
	if err != nil {
		t.Error(err)
	}
	expected := word(2)
	if actual.exHeaderSize != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exHeaderSize)
	}
}

func TestParseHeaderInitSS(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, err := parseHeader(reader)
	if err != nil {
		t.Error(err)
	}
	expected := word(0x0001)
	if actual.exInitSS != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitSS)
	}
}

func TestParseHeaderInitSP(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, err := parseHeader(reader)
	if err != nil {
		t.Error(err)
	}
	expected := word(0x1000)
	if actual.exInitSP != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitSP)
	}
}

func TestParseHeaderInitIP(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, err := parseHeader(reader)
	if err != nil {
		t.Error(err)
	}
	expected := word(0x0100)
	if actual.exInitIP != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitIP)
	}
}

func TestParseHeaderInitCS(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, err := parseHeader(reader)
	if err != nil {
		t.Error(err)
	}
	expected := word(0x0002)
	if actual.exInitCS != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitCS)
	}
}

func TestDecodeInstInt(t *testing.T) {
	// int 21
	var reader io.Reader = bytes.NewReader([]byte{0xcd, 0x21})
	actual, err := decodeInst(reader)
	if err != nil {
		t.Error(err)
	}
	expected := instInt{operand: 0x21}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovAX(t *testing.T) {
	// mov ax,1
	var reader io.Reader = bytes.NewReader([]byte{0xb8, 0x01, 0x00})
	actual, err := decodeInst(reader)
	if err != nil {
		t.Error(err)
	}
	expected := instMov{dest: AX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovCX(t *testing.T) {
	// mov cx,1
	var reader io.Reader = bytes.NewReader([]byte{0xb9, 0x01, 0x00})
	actual, err := decodeInst(reader)
	if err != nil {
		t.Error(err)
	}
	expected := instMov{dest: CX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeShlAX(t *testing.T) {
	// shl ax,1
	var reader io.Reader = bytes.NewReader([]byte{0xc1, 0xe0, 0x01})
	actual, err := decodeInst(reader)
	if err != nil {
		t.Error(err)
	}
	expected := instShl{register: AX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeShlCX(t *testing.T) {
	// shl cx,1
	var reader io.Reader = bytes.NewReader([]byte{0xc1, 0xe1, 0x01})
	actual, err := decodeInst(reader)
	if err != nil {
		t.Error(err)
	}
	expected := instShl{register: CX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeAddAX(t *testing.T) {
	// add ax,1
	var reader io.Reader = bytes.NewReader([]byte{0x83, 0xc0, 0x01})
	actual, err := decodeInst(reader)
	if err != nil {
		t.Error(err)
	}
	expected := instAdd{dest: AX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeAddCX(t *testing.T) {
	// add ax,1
	var reader io.Reader = bytes.NewReader([]byte{0x83, 0xc1, 0x01})
	actual, err := decodeInst(reader)
	if err != nil {
		t.Error(err)
	}
	expected := instAdd{dest: CX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func (code machineCode) withMov() machineCode {
	// mov ax,1
	mov := []byte{0xb8, 0x01, 0x00}
	return append(code, mov...)
}

func TestRunExe(t *testing.T) {
	actual, err := RunExe(bytes.NewReader(rawHeader().withMov()))
	if err != nil {
		t.Error(err)
	}
	if actual.ax != 0x0001 {
		t.Errorf("register ax is expected to be 0x%04x but actual 0x%04x", 0x0001, actual.ax)
	}
}

func (code machineCode) withShl() machineCode {
	// shl ax,1
	shl := []byte{0xc1, 0xe0, 0x01}
	return append(code, shl...)
}

func TestShlExe(t *testing.T) {
	actual, err := RunExe(bytes.NewReader(rawHeader().withMov().withShl()))
	if err != nil {
		t.Error(err)
	}
	if actual.ax != 0x0002 {
		t.Errorf("register ax is expected to be 0x%04x but actual 0x%04x", 0x0002, actual.ax)
	}
}

func (code machineCode) withAdd() machineCode {
	// add ax,1
	add := []byte{0x83, 0xc0, 0x03}
	return append(code, add...)
}

func TestAddExe(t *testing.T) {
	actual, err := RunExe(bytes.NewReader(rawHeader().withAdd()))
	if err != nil {
		t.Error(err)
	}
	if actual.ax != 0x0003 {
		t.Errorf("register ax is expected to be 0x%04x but actual 0x%04x", 0x0003, actual.ax)
	}
}

func TestInt21_4c_ax(t *testing.T) {
	// exit status 1
	b := rawHeader()
	b = append(b, []byte{0xb8, 0x4c, 0x00}...) // mov ax,4ch
	b = append(b, []byte{0xc1, 0xe0, 0x08}...) // shl ax,8
	b = append(b, []byte{0x83, 0xc0, 0x01}...) // add ax,01h
	b = append(b, []byte{0xcd, 0x21}...)       // int 21h

	var exitCode uint8;
	intHandlers := make(intHandlers)
	intHandlers[0x4c] = func(s state) error {
		exitCode = s.al()
		return nil
	}

	actual, err := runExeWithCustomIntHandlers(bytes.NewReader(b), intHandlers)
	if err != nil {
		t.Error(err)
	}

	if actual.ax != 0x4c01 {
		t.Errorf("ax is illegal: %04x", actual.ax)
	}
	if actual.al() != 0x01 {
		t.Errorf("al is illegal: %02x", actual.al())
	}
	if actual.ah() != 0x4c {
		t.Errorf("ah is illegal: %02x", actual.ah())
	}
	if exitCode != 0x01 {
		t.Errorf("exitCode is expected to be %02x but %02x", 0x01, exitCode)
	}
}

func TestInt21_4c_cx(t *testing.T) {
	// exit status 1
	b := rawHeader()
	b = append(b, []byte{0xb9, 0x4c, 0x00}...) // mov cx,4ch
	b = append(b, []byte{0xc1, 0xe1, 0x08}...) // shl cx,8
	b = append(b, []byte{0x83, 0xc1, 0x01}...) // add cx,01h
	b = append(b, []byte{0x8b, 0xc1}...)       // mov ax,cx
	b = append(b, []byte{0xcd, 0x21}...)       // int 21h

	var exitCode uint8;
	intHandlers := make(intHandlers)
	intHandlers[0x4c] = func(s state) error {
		exitCode = s.al()
		return nil
	}

	actual, err := runExeWithCustomIntHandlers(bytes.NewReader(b), intHandlers)
	if err != nil {
		t.Error(err)
	}

	if actual.ax != 0x4c01 {
		t.Errorf("ax is illegal: %04x", actual.ax)
	}
	if actual.al() != 0x01 {
		t.Errorf("al is illegal: %02x", actual.al())
	}
	if actual.ah() != 0x4c {
		t.Errorf("ah is illegal: %02x", actual.ah())
	}
	if exitCode != 0x01 {
		t.Errorf("exitCode is expected to be %02x but %02x", 0x01, exitCode)
	}
}
