package x86_emulator

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

type machineCode []byte

// address

func TestRealAddress(t *testing.T) {
	// maximum address of real mode
	x := newAddress(0xffff, 0xffff)
	if x.realAddress() != 0x10ffef {
		t.Errorf("expected %06x but actual %06x", 0x10ffef, x.realAddress())
	}
}

// operand

func TestNewImm8(t *testing.T) {
	fixtures := [][]byte{
		[]byte{0x7f},
		[]byte{0xff},
	}
	expecteds := []int8{
		127,
		-1,
	}
	for i := 0; i < len(fixtures); i++ {
		var actual imm8
		var err error
		if actual, err = newImm8(bytes.NewReader(fixtures[i])); err != nil {
			t.Errorf("%+v", err)
		}
		expected := expecteds[i]
		if actual.value != expected {
			t.Errorf("expect %d as fixtures[%d] but actual %d", expected, i, actual)
		}
	}
}

// decode

func TestDecodeInstInt(t *testing.T) {
	// int 21
	var reader io.Reader = bytes.NewReader([]byte{0xcd, 0x21})
	actual, _, _, err := decodeInst(reader)
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
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	dest := reg16{value: AX}
	src := imm16{value: 0x0001}
	expected := instMov{dest: dest, src: src}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovCX(t *testing.T) {
	// mov cx,1
	var reader io.Reader = bytes.NewReader([]byte{0xb9, 0x01, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	dest := reg16{value: CX}
	src := imm16{value: 0x0001}
	expected := instMov{dest: dest, src: src}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovDs(t *testing.T) {
	// mov ds,ax
	var reader io.Reader = bytes.NewReader([]byte{0x8e, 0xd8})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovSRegReg{dest: DS, src: AX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovReg8Imm8(t *testing.T) {
	// mov ah,09h
	var reader io.Reader = bytes.NewReader([]byte{0xb4, 0x09})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	dest := reg8{value: AH}
	src := imm8{value: 0x09}
	expected := instMov{dest: dest, src: src}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovMem16Reg16WithSegmentOverride(t *testing.T) {
	// mov word ptr es:0038, bx
	var reader io.Reader = bytes.NewReader([]byte{0x26, 0x89, 0x1e, 0x38, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovMem16Reg16{offset: 0x0038, src: BX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovReg16Mem16WithSegmentOverride(t *testing.T) {
	// mov word ptr es:0038, bx
	var reader io.Reader = bytes.NewReader([]byte{0x26, 0x8b, 0x16, 0xb0, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	dest := reg16{value: DX}
	src := mem16Disp16{offset: 0x00b0}
	expected := instMov{dest: dest, src: src}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovMem16SregWithSegmentOverride(t *testing.T) {
	// mov word ptr es:0032,ds
	var reader io.Reader = bytes.NewReader([]byte{0x26, 0x8c, 0x1e, 0x32, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovMem16Sreg{offset: 0x0032, src: DS}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovAxMoffs16WithSegmentOverride(t *testing.T) {
	// mov ax,word ptr es:0032
	var reader io.Reader = bytes.NewReader([]byte{0x26, 0xa1, 0x32, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovReg16Mem16{dest: AX, offset: 0x0032}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovMoffs16AlWithSegmentOverride(t *testing.T) {
	// mov byte ptr es:0034,al
	var reader io.Reader = bytes.NewReader([]byte{0x26, 0xa2, 0x34, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	dest := mem8Disp16{offset: 0x0034}
	src := reg8{value: AL}
	expected := instMov{dest: dest, src: src}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovMMoffs16Ax(t *testing.T) {
	// mov word ptr 0042,ax
	var reader io.Reader = bytes.NewReader([]byte{0xa3, 0x42, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovMem16Reg16{offset: 0x0042, src: AX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovReg8WithDisp(t *testing.T) {
	// mov cl, byte ptr -01[di]
	var reader io.Reader = bytes.NewReader([]byte{0x8a, 0x4d, 0xff})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	dest := reg8{value: CL}
	src := mem8BaseDisp8{base: DI, disp8: -1}
	expected := instMov{dest: dest, src: src}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovMem16Imm16(t *testing.T) {
	// mov word ptr 0x005e,0x2000
	var reader io.Reader = bytes.NewReader([]byte{0xc7, 0x06, 0x5e, 0x00, 0x00, 0x20})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovMem16Imm16{offset: 0x005e, imm16: 0x2000}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovMem16Disp8Imm16(t *testing.T) {
	// mov word ptr -2[bp], 0x0002
	var reader io.Reader = bytes.NewReader([]byte{0xc7, 0x46, 0xfe, 0x02, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovMem16Disp8Imm16{base: BP, disp8: -2, imm16: 0x0002}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovMem16Disp8Reg16(t *testing.T) {
	// mov word ptr -4[bp], ax
	var reader io.Reader = bytes.NewReader([]byte{0x89, 0x46, 0xfc})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovMem16Disp8Reg16{base: BP, disp8: -4, src: AX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeShlAX(t *testing.T) {
	// shl ax,1
	var reader io.Reader = bytes.NewReader([]byte{0xc1, 0xe0, 0x01})
	actual, _, _, err := decodeInst(reader)
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
	actual, _, _, err := decodeInst(reader)
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
	actual, _, _, err := decodeInst(reader)
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
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instAdd{dest: CX, imm: 0x0001}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeSubReg16Imm8(t *testing.T) {
	// sub ax,2
	var reader io.Reader = bytes.NewReader([]byte{0x83, 0xec, 0x02})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instSubReg16Imm8{dest: SP, imm: 0x0002}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeSubReg16Reg16(t *testing.T) {
	// sub cx,ax
	var reader io.Reader = bytes.NewReader([]byte{0x2b, 0xc8})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instSubReg16Reg16{dest: CX, src: AX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeSubReg8Reg8(t *testing.T) {
	// sub al,al
	var reader io.Reader = bytes.NewReader([]byte{0x2a, 0xc0})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instSubReg8Reg8{dest: AL, src: AL}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeSubReg16Imm16(t *testing.T) {
	// sub sp,0x0002
	var reader io.Reader = bytes.NewReader([]byte{0x81, 0xec, 0x02, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instSubReg16Imm16{dest: SP, imm16: 0x0002}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeLeaDx(t *testing.T) {
	// lea dx,msg
	var reader io.Reader = bytes.NewReader([]byte{0x8d, 0x16, 0x02, 0x00}) // 0b00010110
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instLea{dest: DX, address: 0x0002}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeLeaReg16Disp8(t *testing.T) {
	// lea SI,-1[DI]
	var reader io.Reader = bytes.NewReader([]byte{0x8d, 0x75, 0xff})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instLeaReg16Disp8{dest: SI, base: DI, disp8: -1}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodePushGeneralRegisters(t *testing.T) {
	// push ax, cx, dx, bx, sp, bp, si, di
	var readers = []io.Reader{
		bytes.NewReader([]byte{0x50}),
		bytes.NewReader([]byte{0x51}),
		bytes.NewReader([]byte{0x52}),
		bytes.NewReader([]byte{0x53}),
		bytes.NewReader([]byte{0x54}),
		bytes.NewReader([]byte{0x55}),
		bytes.NewReader([]byte{0x56}),
		bytes.NewReader([]byte{0x57}),
	}
	var expected = []instPush{
		instPush{src: AX},
		instPush{src: CX},
		instPush{src: DX},
		instPush{src: BX},
		instPush{src: SP},
		instPush{src: BP},
		instPush{src: SI},
		instPush{src: DI},
	}

	for i := 0; i < len(readers); i++ {
		actual, _, _, err := decodeInst(readers[i])
		if err != nil {
			t.Errorf("%+v", err)
		}
		if actual != expected[i] {
			t.Errorf("expected %v but actual %v", expected[i], actual)
		}
	}
}

func TestDecodePushDs(t *testing.T) {
	// push ds
	var reader io.Reader = bytes.NewReader([]byte{0x1e})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instPushSreg{src: DS}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodePopGeneralRegisters(t *testing.T) {
	// pop ax, cx, dx, bx, sp, bp, si, di
	var readers = []io.Reader{
		bytes.NewReader([]byte{0x58}),
		bytes.NewReader([]byte{0x59}),
		bytes.NewReader([]byte{0x5a}),
		bytes.NewReader([]byte{0x5b}),
		bytes.NewReader([]byte{0x5c}),
		bytes.NewReader([]byte{0x5d}),
		bytes.NewReader([]byte{0x5e}),
		bytes.NewReader([]byte{0x5f}),
	}
	var expected = []instPop{
		instPop{dest: AX},
		instPop{dest: CX},
		instPop{dest: DX},
		instPop{dest: BX},
		instPop{dest: SP},
		instPop{dest: BP},
		instPop{dest: SI},
		instPop{dest: DI},
	}

	for i := 0; i < len(readers); i++ {
		actual, _, _, err := decodeInst(readers[i])
		if err != nil {
			t.Errorf("%+v", err)
		}
		if actual != expected[i] {
			t.Errorf("expected %v but actual %v", expected[i], actual)
		}
	}
}

func TestDecodePopDs(t *testing.T) {
	// pop ds
	var reader io.Reader = bytes.NewReader([]byte{0x1f})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instPopSreg{dest: DS}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeCall(t *testing.T) {
	// call rel16
	var reader io.Reader = bytes.NewReader([]byte{0xe8, 0xdc, 0xff})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instCall{rel: -36} // 0xffdc
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeCallAbsoluteIndirectMem16(t *testing.T) {
	// call r/m16
	var reader io.Reader = bytes.NewReader([]byte{0xff, 0x16, 0x52, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instCallAbsoluteIndirectMem16{offset: 0x0052}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeRet(t *testing.T) {
	// ret (near return)
	var reader io.Reader = bytes.NewReader([]byte{0xc3})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instRet{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovWithDisp(t *testing.T) {
	// mov ax,[bp+4]
	var reader io.Reader = bytes.NewReader([]byte{0x8b, 0x46, 0x04})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	dest := reg16{value: AX}
	src := mem16BaseDisp8{base: BP, disp8: 4}
	expected := instMov{dest: dest, src: src}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeJmpRel16(t *testing.T) {
	// jmp rel16
	var reader io.Reader = bytes.NewReader([]byte{0xe9, 0x8a, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instJmpRel16{rel: 0x008a}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeJmpRel8(t *testing.T) {
	// jmp rel8
	var reader io.Reader = bytes.NewReader([]byte{0xeb, 0xfd})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instJmpRel16{rel: -3}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeSti(t *testing.T) {
	// sti
	var reader io.Reader = bytes.NewReader([]byte{0xfb})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instSti{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeAndReg8Imm8(t *testing.T) {
	// and r/m8 imm8
	var reader io.Reader = bytes.NewReader([]byte{0x80, 0xe3, 0xf0})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instAndReg8Imm8{reg: BL, imm8: 0xf0}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeAndMem8Reg8(t *testing.T) {
	// and r/m8,r8
	var reader io.Reader = bytes.NewReader([]byte{0x20, 0x26, 0x5a, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instAndMem8Reg8{offset: 0x005a, reg8: AH}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeAddReg16Reg16(t *testing.T) {
	// add r16,r/m16
	var reader io.Reader = bytes.NewReader([]byte{0x03, 0xdc})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instAddReg16Reg16{dest: BX, src: SP}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeShrReg16_1(t *testing.T) {
	// shr r/m16,1
	var reader io.Reader = bytes.NewReader([]byte{0xd1, 0xea})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instShrReg16Imm{reg: DX, imm: 1}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeShlReg16_1(t *testing.T) {
	// shl r/m16,1
	var reader io.Reader = bytes.NewReader([]byte{0xd1, 0xe3})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instShlReg16Imm{reg: BX, imm: 1}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeCmpWithSegmentOverride(t *testing.T) {
	// cmp es:0036, 0x00
	var reader io.Reader = bytes.NewReader([]byte{0x26, 0x80, 0x3e, 0x36, 0x00, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instCmpMem8Imm8{offset: 0x0036, imm8: 0x00}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeJneRel8(t *testing.T) {
	// jne 0x3d
	var reader io.Reader = bytes.NewReader([]byte{0x75, 0x3d})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instJneRel8{rel8: 0x3d}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeMovReg16Sreg(t *testing.T) {
	// mov ax,es
	var reader io.Reader = bytes.NewReader([]byte{0x8c, 0xc0})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instMovReg16Sreg{dest: AX, src: ES}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeCmpReg16Reg16(t *testing.T) {
	// cmp dx,cx
	var reader io.Reader = bytes.NewReader([]byte{0x3b, 0xd1})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instCmpReg16Reg16{first: DX, second: CX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeCmpAlImm8(t *testing.T) {
	// cmp al,0x03
	var reader io.Reader = bytes.NewReader([]byte{0x3c, 0x03})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instCmpReg8Imm8{reg8: AL, imm8: 0x03}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeCmpReg16Imm16(t *testing.T) {
	// cmp bx,0x0064
	var reader io.Reader = bytes.NewReader([]byte{0x81, 0xfb, 0x64, 0x00})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instCmpReg16Imm16{reg16: BX, imm16: 0x0064}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeJb(t *testing.T) {
	// jb 0x0b
	var reader io.Reader = bytes.NewReader([]byte{0x72, 0x0b})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instJb{rel8: 0x0b}
	if actual != expected {
		t.Errorf("expeted %v but actual %v", expected, actual)
	}
}

func TestDecodeCld(t *testing.T) {
	// cld
	var reader io.Reader = bytes.NewReader([]byte{0xfc})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instCld{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeRepeScasb(t *testing.T) {
	// repe scasb
	var reader io.Reader = bytes.NewReader([]byte{0xf3, 0xae})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instRepeScasb{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeRepeScasw(t *testing.T) {
	// repe scasw
	var reader io.Reader = bytes.NewReader([]byte{0xf3, 0xaf})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instRepeScasw{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeRepMovsb(t *testing.T) {
	// rep movsb
	var reader io.Reader = bytes.NewReader([]byte{0xf3, 0xa4})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instRepMovsb{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeRepStosb(t *testing.T) {
	// rep stosb
	var reader io.Reader = bytes.NewReader([]byte{0xf3, 0xaa})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instRepStosb{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeJe(t *testing.T) {
	// je 0x03
	var reader io.Reader = bytes.NewReader([]byte{0x74, 0x03})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instJeRel8{rel8: 0x03}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeInc(t *testing.T) {
	// inc cx
	var reader io.Reader = bytes.NewReader([]byte{0x41})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instInc{dest: CX}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeStosb(t *testing.T) {
	// stos m8
	var reader io.Reader = bytes.NewReader([]byte{0xaa})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instStosb{}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeDec(t *testing.T) {
	// dec di
	var reader io.Reader = bytes.NewReader([]byte{0x4f})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instDec{dest: DI}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeXorReg16Reg16(t *testing.T) {
	// xor bp,bp
	var reader io.Reader = bytes.NewReader([]byte{0x33, 0xed})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instXorReg16Reg16{dest: BP, src: BP}
	if actual != expected {
		t.Errorf("expected %v but actual %v", expected, actual)
	}
}

func TestDecodeJae(t *testing.T) {
	// jae rel8
	var reader io.Reader = bytes.NewReader([]byte{0x73, 0x16})
	actual, _, _, err := decodeInst(reader)
	if err != nil {
		t.Errorf("%+v", err)
	}
	expected := instJae{rel8: 0x16}
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

func rawHeaderForRunExe() machineCode {
	return []byte{
		0x4d, 0x5a, 0x2b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x01, 0xff, 0xff, 0x01, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestRunExe(t *testing.T) {
	_, _, err := RunExe(bytes.NewReader(rawHeaderForRunExe().withMov().withInt21_4c()))
	if err != nil {
		t.Errorf("%+v", err)
	}
}

func TestInt21_4c_ax(t *testing.T) {
	// exit status 1
	b := rawHeaderForRunExe()
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
	b := rawHeaderForRunExe()
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
	b = append(b, []byte{0xb8, 0x01, 0x00}...)       // mov ax,seg msg
	b = append(b, []byte{0x8e, 0xd8}...)             // mov ds,ax
	b = append(b, []byte{0xb4, 0x09}...)             // mov ah,09h
	b = append(b, []byte{0x8d, 0x16, 0x02, 0x00}...) // lea dx,msg
	b = append(b, []byte{0xcd, 0x21}...)             // int 21h
	b = append(b, []byte{0xb8, 0x00, 0x4c}...)       // mov ax,4c00h
	b = append(b, []byte{0xcd, 0x21}...)             // int 21h
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

func rawHeaderForTestPush() machineCode {
	return []byte{
		0x4d, 0x5a, 0x2b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x01, 0xff, 0xff, 0x01, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestPushAndPop(t *testing.T) {
	b := rawHeaderForTestPush()
	b = append(b, []byte{0xb8, 0x35, 0x10}...) // mov ax, 0x1035
	b = append(b, []byte{0xb9, 0x36, 0x20}...) // mov cx, 0x2036
	b = append(b, []byte{0x50}...)             // push ax
	b = append(b, []byte{0x51}...)             // push cx
	b = append(b, []byte{0x5b}...)             // pop bx
	b = append(b, []byte{0x5a}...)             // pop dx
	b = append(b, []byte{0xb8, 0x00, 0x4c}...) // mov ax,4c00h
	b = append(b, []byte{0xcd, 0x21}...)       // int 21h

	intHandlers := make(intHandlers)

	actual, err := runExeWithCustomIntHandlers(bytes.NewReader(b), intHandlers)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if actual.dx != 0x1035 {
		t.Errorf("expect 0x%04x but 0x%04x", 0x1035, actual.dx)
	}
	if actual.bx != 0x2036 {
		t.Errorf("expect 0x%04x but 0x%04x", 0x2036, actual.bx)
	}
}

// RunExe with sample file

func TestRunExeWithSampleFcall(t *testing.T) {
	file, err := os.Open("sample/fcall.exe")
	if err != nil {
		t.Errorf("%+v", err)
	}
	exitCode, _, err := RunExe(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if exitCode != 7 {
		t.Errorf("expect exitCode to be %d but actual %d", 7, exitCode)
	}
}

func TestRunExeWithSampleFcallp(t *testing.T) {
	file, err := os.Open("sample/fcallp.exe")
	if err != nil {
		t.Errorf("%+v", err)
	}
	exitCode, _, err := RunExe(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if exitCode != 7 {
		t.Errorf("expect exitCode to be %d but actual %d", 7, exitCode)
	}
}

func TestRunExeWithSampleCmain(t *testing.T) {
	file, err := os.Open("sample/cmain.exe")
	if err != nil {
		t.Errorf("%+v", err)
	}
	exitCode, _, err := RunExe(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if exitCode != 0 {
		t.Errorf("expect exitCode to be %d but actual %d", 0, exitCode)
	}
}

// check segment override
func TestRunExeWithSampleSgor(t *testing.T) {
	file, err := os.Open("sample/sgor.exe")
	if err != nil {
		t.Errorf("%+v", err)
	}
	exitCode, state, err := RunExe(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if exitCode != 0 {
		t.Errorf("expect exitCode to be %d but actual %d", 0, exitCode)
	}
	if state.dx != 0x0370 {
		t.Errorf("expect dx as 0x%04x but actual 0x%04x", 0x0370, state.dx)
	}
}

// c main without _exit
func TestRunExeWithSampleCmain2(t *testing.T) {
	file, err := os.Open("sample/cmain2.exe")
	if err != nil {
		t.Errorf("%+v", err)
	}
	// debug = debugT(true)
	exitCode, _, err := RunExe(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if exitCode != 8 {
		t.Errorf("expect exitCode to be %d but actual %d", 8, exitCode)
	}
}

// c main with _exit
func TestRunExeWithSampleCmain5(t *testing.T) {
	file, err := os.Open("sample/cmain5.exe")
	if err != nil {
		t.Errorf("%+v", err)
	}
	// debug = debugT(true)
	exitCode, _, err := RunExe(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if exitCode != 2 {
		t.Errorf("expect exitCode to be %d but actual %d", 8, exitCode)
	}
}

// hello world without libc
func TestRunExeWithSampleHll(t *testing.T) {
	file, err := os.Open("sample/hll.exe")
	if err != nil {
		t.Errorf("%+v", err)
	}
	// debug = debugT(true)
	exitCode, _, err := RunExe(file)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if exitCode != 0 {
		t.Errorf("expect exitCode to be %d but actual %d", 0, exitCode)
	}
}
