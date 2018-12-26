package x86_emulator

import (
	"bytes"
	"io"
	"testing"
)

func rawHeader() machineCode {
	// 32 bytes + 3 bytes (since initIP is set as 0x0003)
	return []byte{
		0x4d, 0x5a, 0x2b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x01, 0xff, 0xff, 0x01, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
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
	expected := word(0x0003)
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

// intialize

func rawHeaderForTestInitilization() []byte {
	return []byte{
		0x4d, 0x5a, 0x71, 0x00, 0x01, 0x00, 0x01, 0x00, 0x03, 0x00, 0x01, 0x01, 0xff, 0xff, 0x05, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x0c, 0x00, 0x03, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x15, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestInitialization(t *testing.T) {
	var reader io.Reader = bytes.NewReader(rawHeaderForTestInitilization())
	header, _, err := parseHeader(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	intHandlers := make(intHandlers)
	state := newState(header, intHandlers)

	// check CS
	expectedCS := word(0x0003)
	if state.cs != expectedCS {
		t.Errorf("expected %v but actual %v", expectedCS, state.cs)
	}

	// check IP
	expectedIP := word(0x000c)
	if state.ip != expectedIP {
		t.Errorf("expected %v but actual %v", expectedIP, state.ip)
	}

	// check SS
	expectedSS := word(0x0005)
	if state.ss != expectedSS {
		t.Errorf("expected %v but actual %v", expectedSS, state.ss)
	}

	// check SP
	expectedSP := word(0x1000)
	if state.sp != expectedSP {
		t.Errorf("expected %v but actual %v", expectedSP, state.sp)
	}
}

