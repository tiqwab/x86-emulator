package x86_emulator

import (
	"testing"
	"bytes"
	"io"
	"os"
	"io/ioutil"
	)

type machineCode []byte

func rawHeader() machineCode {
	// 32 bytes
	return []byte{
		0x4d, 0x5a, 0x2b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x01, 0xff, 0xff, 0x01, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestParseHeaderSignature(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := [2]byte{'M', 'Z'}
	if actual.exSignature != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exSignature)
	}
}

func TestParseHeaderSize(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := word(2)
	if actual.exHeaderSize != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exHeaderSize)
	}
}

func TestParseHeaderInitSS(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := word(0x0001)
	if actual.exInitSS != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitSS)
	}
}

func TestParseHeaderInitSP(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := word(0x1000)
	if actual.exInitSP != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitSP)
	}
}

func TestParseHeaderInitIP(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := word(0x0100)
	if actual.exInitIP != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitIP)
	}
}

func TestParseHeaderInitCS(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeader())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := word(0x0002)
	if actual.exInitCS != expected {
		t.Errorf("expected %v but actual %v", expected, actual.exInitCS)
	}
}

// relocation

func rawHeaderWithRelocation() machineCode {
	// 48 bytes
	return []byte{
		0x4d, 0x5a, 0x4f, 0x00, 0x01, 0x00, 0x01, 0x00, 0x03, 0x00, 0x01, 0x01, 0xff, 0xff, 0x02, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestParseHeaderRelocationItems(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeaderWithRelocation())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := word(1)
	if actual.relocationItems != expected {
		t.Errorf("expected %v but actual %v", expected, actual.relocationItems)
	}
}

func TestParseHeaderRelocationOffset(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeaderWithRelocation())
	actual, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := word(0x0020)
	if actual.relocationTableOffset != expected {
		t.Errorf("expected %v but actual %v", expected, actual.relocationTableOffset)
	}
}

// decode

func TestDecodeInstInt(t *testing.T) {
	// int 21
	var reader io.Reader = bytes.NewReader([]byte{0xcd, 0x21})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instInt{operand: 0x21}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovAX(t *testing.T) {
	// mov ax,1
	var reader io.Reader = bytes.NewReader([]byte{0xb8, 0x01, 0x00})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMov{dest: AX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovCX(t *testing.T) {
	// mov cx,1
	var reader io.Reader = bytes.NewReader([]byte{0xb9, 0x01, 0x00})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMov{dest: CX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeShlAX(t *testing.T) {
	// shl ax,1
	var reader io.Reader = bytes.NewReader([]byte{0xc1, 0xe0, 0x01})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instShl{register: AX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeShlCX(t *testing.T) {
	// shl cx,1
	var reader io.Reader = bytes.NewReader([]byte{0xc1, 0xe1, 0x01})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instShl{register: CX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeAddAX(t *testing.T) {
	// add ax,1
	var reader io.Reader = bytes.NewReader([]byte{0x83, 0xc0, 0x01})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instAdd{dest: AX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeAddCX(t *testing.T) {
	// add ax,1
	var reader io.Reader = bytes.NewReader([]byte{0x83, 0xc1, 0x01})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instAdd{dest: CX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovDs(t *testing.T) {
	// mov ds,ax
	var reader io.Reader = bytes.NewReader([]byte{0x8e, 0xd8})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovSRegReg{dest: DS, src: AX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovAh(t *testing.T) {
	// mov ah,09h
	var reader io.Reader = bytes.NewReader([]byte{0xb4, 0x09})
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovB{dest: AH, imm: 0x09}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeLeaDx(t *testing.T) {
	// lea dx,msg
	var reader io.Reader = bytes.NewReader([]byte{0x8d, 0x16, 0x02, 0x00}) // 0b00010110
	actual, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instLea{dest: DX, address: 0x0002}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

// run

func (code machineCode) withMov() machineCode {
	// mov ax,1
	mov := []byte{0xb8, 0x01, 0x00}
	return append(code, mov...)
}

func (code machineCode) withInt21_4c() machineCode {
	// mov ah, 4c
	// int 21h
	int21 := []byte{0xb4, 0x4c, 0xcd, 0x21}
	return append(code, int21...)
}

func TestRunExe(t *testing.T) {
	_, _, err := RunExe(bytes.NewReader(rawHeader().withMov().withInt21_4c()))
	if err != nil {
		t.Errorf("%+v", err)
	}
}

func TestInt21_4c_ax(t *testing.T) {
	// exit status 1
	b := rawHeader()
	b = append(b, []byte{0xb8, 0x4c, 0x00}...) // mov ax,4ch
	b = append(b, []byte{0xc1, 0xe0, 0x08}...) // shl ax,8
	b = append(b, []byte{0x83, 0xc0, 0x01}...) // add ax,01h
	b = append(b, []byte{0xcd, 0x21}...)       // int 21h

	intHandlers := make(intHandlers)

	actual, err := runExeWithCustomIntHandlers(bytes.NewReader(b), intHandlers)
	if err != nil {
		t.Errorf("%+v", err)
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
	if actual.exitCode != 0x01 {
		t.Errorf("exitCode is expected to be %02x but %02x", 0x01, actual.exitCode)
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

	intHandlers := make(intHandlers)

	actual, err := runExeWithCustomIntHandlers(bytes.NewReader(b), intHandlers)
	if err != nil {
		t.Errorf("%+v", err)
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
	if actual.exitCode != 0x01 {
		t.Errorf("exitCode is expected to be %02x but %02x", 0x01, actual.exitCode)
	}
}

// for print systemcall

func rawHeaderForTestInt21_09() machineCode {
	// assume that there is one relocation
	// (1) relocation items
	// (2) item of relocation table
	return []byte{
	//                                      <--(1)--->
		0x4d, 0x5a, 0x4f, 0x00, 0x01, 0x00, 0x01, 0x00, 0x03, 0x00, 0x01, 0x01, 0xff, 0xff, 0x02, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//  <--(2)--->
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestInt21_09(t *testing.T) {
	b := rawHeaderForTestInt21_09()
	b = append(b, []byte{0xb8, 0x01, 0x00}...) // mov ax,seg msg
	b = append(b, []byte{0x8e, 0xd8}...) // mov ds,ax
	b = append(b, []byte{0xb4, 0x09}...) // mov ah,09h
	b = append(b, []byte{0x8d, 0x16, 0x02, 0x00}...) // lea dx,msg
	b = append(b, []byte{0xcd, 0x21}...) // int 21h
	b = append(b, []byte{0xb8, 0x00, 0x4c}...) // mov ax,4c00h
	b = append(b, []byte{0xcd, 0x21}...) // int 21h
	b = append(b, []byte("Hello world!$")...)

	tempFile, err := ioutil.TempFile("", "TestInt21_09")
	if err != nil {
		t.Errorf("%+v", err)
	}
	defer os.Remove(tempFile.Name())

	intHandlers := make(intHandlers)
	intHandlers[0x09] = func(s *state, m *memory) error {
		originalStdout := os.Stdout
		os.Stdout = tempFile
		err = intHandler09(s, m)
		os.Stdout = originalStdout
		return err
	}

	_, err = runExeWithCustomIntHandlers(bytes.NewReader(b), intHandlers)
	if err != nil {
		t.Errorf("%+v", err)
	}

	tempFile.Sync()
	tempFile.Seek(0, 0)
	output, err := ioutil.ReadAll(tempFile)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if string(output) != "Hello world!" {
		t.Errorf("expect output \"%s\" but \"%s\"", "Hello world!", string(output))
	}

	if err = tempFile.Close(); err != nil {
		t.Errorf("%+v", err)
	}
}
