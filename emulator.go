package x86_emulator

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
)

// ref1. https://en.wikibooks.org/wiki/X86_Assembly/Machine_Language_Conversion
// ref2. https://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-instruction-set-reference-manual-325383.pdf

// for debug log
type debugT bool
var debug = debugT(false)
func (d debugT) printf(format string, args ...interface{}) {
	if d {
		log.Printf(format, args...)
	}
}

type word uint16
type dword uint32
type exitCode uint8

type segmentOverride struct {
	sreg registerS
}

// --- memory

type memory struct {
	loadModule []byte
	memorySize int
}

type address struct {
	seg uint16
	offset uint16
}

func newAddress(seg uint16, offset uint16) *address {
	return &address{seg: seg, offset: offset}
}

func newAddressFromWord(seg word, offset word) *address {
	return &address{seg: uint16(seg), offset: uint16(offset)}
}

func (address *address) plus(x int) {
	address.offset = uint16(int(address.offset) + x)
}

func (address *address) plus8(x int8) {
	address.offset = uint16(int16(address.offset) + int16(x))
}

func (address *address) plus16(x int16) {
	address.offset = uint16(int16(address.offset) + x)
}

func (address *address) realAddress() int {
	return int(address.seg) << 4 + int(address.offset)
}

// Prepare memory whose size is same as load module
func newMemory(loadModule []byte) *memory {
	loadModuleSize := len(loadModule)
	m := make([]byte, loadModuleSize)
	for i := 0; i < loadModuleSize; i++ {
		m[i] = loadModule[i]
	}
	return &memory{loadModule: m, memorySize: loadModuleSize}
}

// TODO: How to calculate the necessary stack size?
func newMemoryFromHeader(loadModule []byte, header *header) *memory {
	loadModuleSize := len(loadModule)
	stackMaxAddress := newAddressFromWord(header.exInitSS, header.exInitSP)
	stackSize := stackMaxAddress.realAddress()
	memorySize := loadModuleSize + stackSize
	m := make([]byte, memorySize)
	for i := 0; i < loadModuleSize; i++ {
		m[i] = loadModule[i]
	}
	return &memory{loadModule: m, memorySize: memorySize}
}

func (memory *memory) readBytes(at *address, n int) ([]byte, error) {
	if at.realAddress() + (n-1) >= memory.memorySize {
		return nil, fmt.Errorf("illegal address: 0x%05x", at)
	}

	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		buf[i] = memory.loadModule[at.realAddress()+i]
	}
	at.offset += uint16(n)
	return buf, nil
}

func (memory *memory) readByte(at *address) (byte, error) {
	b, err := memory.readBytes(at, 1)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read byte")
	}
	return b[0], nil
}

func (memory *memory) readWord(at *address) (word, error) {
	buf, err := memory.readBytes(at, 2)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read word")
	}
	return word(buf[1]) << 8 + word(buf[0]), nil
}

func (memory *memory) readInt8(at *address) (int8, error) {
	var v int8
	bs, err := memory.readBytes(at, 1)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read int8")
	}
	buf := bytes.NewReader(bs)
	err = binary.Read(buf, binary.LittleEndian, &v)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse as int8")
	}
	return v, nil
}

func (memory *memory) readInt16(at *address) (int16, error) {
	var v int16
	bs, err := memory.readBytes(at, 2)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read int16")
	}
	buf := bytes.NewReader(bs)
	err = binary.Read(buf, binary.LittleEndian, &v)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse as int16")
	}
	return v, nil
}

func (memory *memory) writeByte(at *address, b byte) error {
	realAddress := at.realAddress()
	if realAddress >= memory.memorySize {
		return fmt.Errorf("illegal address: 0x%05x", at)
	}
	memory.loadModule[realAddress] = b
	return nil
}

func (memory *memory) writeWord(at *address, w word) error {
	realAddress := at.realAddress()
	if realAddress >= memory.memorySize {
		return fmt.Errorf("illegal address: 0x%05x", at)
	}
	low := byte(w & 0x00ff)
	high := byte((w & 0xff00) >> 8)
	memory.loadModule[realAddress] = low
	memory.loadModule[realAddress+1] = high
	return nil
}

// --- registers

type registerW uint8
type registerB uint8
type registerS uint8

const (
	// ref2. 3.1.1.1
	AX = registerW(0)
	CX = registerW(1)
	DX = registerW(2)
	BX = registerW(3)
	SP = registerW(4)
	BP = registerW(5)
	SI = registerW(6)
	DI = registerW(7)
	AL = registerB(0)
	CL = registerB(1)
	DL = registerB(2)
	BL = registerB(3)
	AH = registerB(4)
	CH = registerB(5)
	DH = registerB(6)
	BH = registerB(7)
	ES = registerS(0)
	CS = registerS(1)
	SS = registerS(2)
	DS = registerS(3)
	FS = registerS(4)
	GS = registerS(5)
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
	default:
		return 0, errors.Errorf("illegal number for registerW: %d", x)
	}
}

func toRegisterB(x uint8) (registerB, error) {
	switch x {
	case 0:
		return AL, nil
	case 1:
		return CL, nil
	case 2:
		return DL, nil
	case 3:
		return BL, nil
	case 4:
		return AH, nil
	case 5:
		return CH, nil
	case 6:
		return DH, nil
	case 7:
		return BH, nil
	default:
		return 0, errors.Errorf("illegal number for registerW: %d", x)
	}
}

func toRegisterS(x uint8) (registerS, error) {
	switch x {
	case 0:
		return ES, nil
	case 1:
		return CS, nil
	case 2:
		return SS, nil
	case 3:
		return DS, nil
	case 4:
		return FS, nil
	case 5:
		return GS, nil
	default:
		return 0, errors.Errorf("illegal number for registerS:%d", x)
	}
}

// --- operand

type operand interface {
	read(state state, memory *memory) (int, error)
	write(value int, state state, memory *memory) (state, error) // FIXME: state passsed as pointer (use it as mutable)
}

type operandAddressing interface {
	operand
	address(state state) (*address, error)
}

type imm8 struct {
	value int8
}

func newImm8(r io.Reader) (imm8, error) {
	var v int8
	err := binary.Read(r, binary.LittleEndian, &v)
	return imm8{value: v}, err
}

func (imm8 imm8) read(s state, m *memory) (int, error) {
	return int(imm8.value), nil
}

func (imm8 imm8) write(v int, s state, m *memory) (state, error) {
	return s, errors.Errorf("cannot write to imm8")
}

type imm16 struct {
	value int16
}

func newImm16(r io.Reader) (imm16, error) {
	var v int16
	err := binary.Read(r, binary.LittleEndian, &v)
	return imm16{value: v}, err
}

func (imm16 imm16) read(s state, m *memory) (int, error) {
	return int(imm16.value), nil
}

func (imm16 imm16) write(v int, s state, m *memory) (state, error) {
	return s, errors.Errorf("cannot write to imm8")
}

type reg8 struct {
	value registerB
}

func newReg8(b byte) (reg8, error) {
	reg, err := toRegisterB(b)
	return reg8{value: reg}, err
}

func (reg8 reg8) read(s state, m *memory) (int, error) {
	v, err := s.readByteGeneralReg(reg8.value)
	return int(v), err
}

func (reg8 reg8) write(v int, s state, m *memory) (state, error) {
	return s.writeByteGeneralReg(reg8.value, uint8(v))
}

type reg16 struct {
	value registerW
}

func newReg16(b byte) (reg16, error) {
	reg, err := toRegisterW(b)
	return reg16{value: reg}, err
}

func (reg16 reg16) read(s state, m *memory) (int, error) {
	v, err := s.readWordGeneralReg(reg16.value)
	return int(v), err
}

func (reg16 reg16) write(v int, s state, m *memory) (state, error) {
	return s.writeWordGeneralReg(reg16.value, word(v))
}

// [reg] + disp8 as byte
type mem8BaseDisp8 struct {
	base registerW // it should be SI, DI, BP, or BX in x86 as shown in Table 2-1. 16-Bit Addressing Forms with the ModR/M Byte
	disp8 int8
}

func (operand mem8BaseDisp8) read(s state, m *memory) (int, error) {
	address, err := operand.address(s)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read mem8BaseDisp8")
	}
	v, err := m.readInt8(address)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read mem8BaseDisp8")
	}
	return int(v), nil
}

func (operand mem8BaseDisp8) write(v int, s state, m *memory) (state, error) {
	address, err := operand.address(s)
	if err != nil {
		return s, errors.Wrap(err, "failed to write to mem8BaseDisp8")
	}
	err = m.writeByte(address, byte(v))
	if err != nil {
		return s, errors.Wrap(err, "failed to write to mem8BaseDisp8")
	}
	return s, nil
}

func (operand mem8BaseDisp8) address(s state) (*address, error) {
	return s.addressFromBaseAndDisp(operand.base, int(operand.disp8))
}

// [disp16] as byte
type mem8Disp16 struct {
	offset word // this can be minus?
}

func (operand mem8Disp16) read(s state, m *memory) (int, error) {
	address, _ := operand.address(s)
	v, err := m.readInt8(address)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read mem8Disp16")
	}
	return int(v), nil
}

func (operand mem8Disp16) write(v int, s state, m *memory) (state, error) {
	address, _ := operand.address(s)
	err := m.writeByte(address, byte(v))
	if err != nil {
		return s, errors.Wrap(err, "failed to write to mem8BaseDisp8")
	}
	return s, nil
}

func (operand mem8Disp16) address(s state) (*address, error) {
	address := newAddressFromWord(s.ds, operand.offset)
	return address, nil
}

// [reg] + disp8 as word
type mem16BaseDisp8 struct {
	base registerW // it should be SI, DI, BP, or BX in x86 as shown in Table 2-1. 16-Bit Addressing Forms with the ModR/M Byte
	disp8 int8
}

func (operand mem16BaseDisp8) read(s state, m *memory) (int, error) {
	address, err := operand.address(s)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read mem8BaseDisp8")
	}
	v, err := m.readInt16(address)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read mem8BaseDisp8")
	}
	return int(v), nil
}

func (operand mem16BaseDisp8) write(v int, s state, m *memory) (state, error) {
	address, err := operand.address(s)
	if err != nil {
		return s, errors.Wrap(err, "failed to write to mem8BaseDisp8")
	}
	err = m.writeWord(address, word(v))
	if err != nil {
		return s, errors.Wrap(err, "failed to write to mem8BaseDisp8")
	}
	return s, nil
}

func (operand mem16BaseDisp8) address(s state) (*address, error) {
	return s.addressFromBaseAndDisp(operand.base, int(operand.disp8))
}

// [disp16] as word
type mem16Disp16 struct {
	offset word // this can be minus?
}

func (operand mem16Disp16) read(s state, m *memory) (int, error) {
	address, _ := operand.address(s)
	v, err := m.readInt16(address)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read mem8Disp16")
	}
	return int(v), nil
}

func (operand mem16Disp16) write(v int, s state, m *memory) (state, error) {
	address, _ := operand.address(s)
	err := m.writeWord(address, word(v))
	if err != nil {
		return s, errors.Wrap(err, "failed to write to mem8BaseDisp8")
	}
	return s, nil
}

func (operand mem16Disp16) address(s state) (*address, error) {
	address := newAddressFromWord(s.ds, operand.offset)
	return address, nil
}

// sreg
type sreg struct {
	value registerS
}

func newSreg(b byte) (operand, error) {
	reg, err := toRegisterS(b)
	return sreg{value: reg}, err
}

func (operand sreg) read(s state, m *memory) (int, error) {
	v, err :=  s.readWordSreg(operand.value)
	return int(v), err
}

func (operand sreg) write(v int, s state, m *memory) (state, error) {
	return s.writeWordSreg(operand.value, word(v))
}

// --- instruction

type instInt struct {
	operand uint8
}

type instMov struct {
	dest operand
	src operand
}

type instShl struct {
	dest operand
	src operand
}

type instShr struct {
	dest operand
	src operand
}

type instSub struct {
	dest operand
	src operand
}

type instLea struct {
	dest operand
	src operandAddressing
}

type instPush struct {
	src registerW
}

type instPushSreg struct {
	src registerS
}

type instPop struct {
	dest registerW
}

type instPopSreg struct {
	dest registerS
}

type instCall struct {
	rel int16
}

type instCallAbsoluteIndirectMem16 struct {
	offset word
}

type instRet struct {

}

type instJmpRel16 struct {
	rel int16
}

type instSti struct {

}

type instAnd struct {
	dest operand
	src operand
}

type instAdd struct {
	dest operand
	src operand
}

type instCmp struct {
	dest operand
	src operand
}

type instJneRel8 struct {
	rel8 int8
}

type instJb struct {
	rel8 int8
}

type instCld struct {

}

type instRepeScasb struct {

}

type instRepeScasw struct {

}

type instRepMovsb struct {

}

type instRepStosb struct {

}

type instStosb struct {

}

type instJeRel8 struct {
	rel8 int8
}

type instInc struct {
	dest registerW
}

type instDec struct {
	dest registerW
}

type instXor struct {
	dest operand
	src operand
}

type instJae struct {
	rel8 int8
}

// --- ModR/M
// Symbols such as Eb, Gb come from Table A-2. One-byte Opcode Map

type modRM struct {
	mod byte
	reg byte
	rm byte
}

func newModRM(at *address, memory *memory) (modRM, error) {
	mod, reg, rm, err := decodeModRegRM(at, memory)
	return modRM{mod: mod, reg: reg, rm: rm}, err
}

func (modRM modRM) getEb(address *address, memory *memory) (operand, error) {
	switch modRM.mod {
	case 0:
		switch modRM.rm {
		case 6:
			offset, err := memory.readWord(address)
			if err != nil {
				return nil, errors.Wrap(err, "failed to getEb")
			}
			return mem8Disp16{offset: offset}, nil
		default:
			return nil, errors.Errorf("illegal or not yet implemeted for rm: %d", modRM.rm)
		}
	case 1:
		disp8, err := memory.readInt8(address)
		if err != nil {
			return nil, errors.Wrap(err, "failed to getEb")
		}
		switch modRM.rm {
		case 4:
			return mem8BaseDisp8{base: SI, disp8: disp8}, nil
		case 5:
			return mem8BaseDisp8{base: DI, disp8: disp8}, nil
		case 6:
			return mem8BaseDisp8{base: BP, disp8: disp8}, nil
		case 7:
			return mem8BaseDisp8{base: BX, disp8: disp8}, nil
		default:
			return nil, errors.Errorf("illegal or not yet implemeted for rm: %d", modRM.rm)
		}
	case 3:
		return newReg8(modRM.rm)
	default:
		return nil, errors.Errorf("illegal or not yet implemented for mod: %d", modRM.mod)
	}
}

func (modRM modRM) getGb() (operand, error) {
	return newReg8(modRM.reg)
}

func (modRM modRM) getEv(address *address, memory *memory) (operand, error) {
	switch modRM.mod {
	case 0:
		switch modRM.rm {
		case 6:
			offset, err := memory.readWord(address)
			if err != nil {
				return nil, errors.Wrap(err, "failed to getEv")
			}
			return mem16Disp16{offset: offset}, nil
		default:
			return nil, errors.Errorf("illegal or not yet implemeted for rm: %d", modRM.rm)
		}
	case 1:
		disp8, err := memory.readInt8(address)
		if err != nil {
			return nil, errors.Wrap(err, "failed to getEv")
		}
		switch modRM.rm {
		case 4:
			return mem16BaseDisp8{base: SI, disp8: disp8}, nil
		case 5:
			return mem16BaseDisp8{base: DI, disp8: disp8}, nil
		case 6:
			return mem16BaseDisp8{base: BP, disp8: disp8}, nil
		case 7:
			return mem16BaseDisp8{base: BX, disp8: disp8}, nil
		default:
			return nil, errors.Errorf("illegal or not yet implemeted for rm: %d", modRM.rm)
		}
	case 3:
		return newReg16(modRM.rm)
	default:
		return nil, errors.Errorf("illegal or not yet implemented for mod: %d", modRM.mod)
	}
}

func (modRM modRM) getEw(address *address, memory *memory) (operand, error) {
	return modRM.getEv(address, memory)
}

func (modRM modRM) getGv() (operand, error) {
	return newReg16(modRM.reg)
}

func (modRM modRM) getSw() (operand, error) {
	return newSreg(modRM.reg)
}

// based on mem8, but size is not necessary
func (modRM modRM) getM(address *address, memory *memory) (operandAddressing, error) {
	switch modRM.mod {
	case 0:
		switch modRM.rm {
		case 6:
			offset, err := memory.readWord(address)
			if err != nil {
				return nil, errors.Wrap(err, "failed to getEb")
			}
			return mem8Disp16{offset: offset}, nil
		default:
			return nil, errors.Errorf("illegal or not yet implemeted for rm: %d", modRM.rm)
		}
	case 1:
		disp8, err := memory.readInt8(address)
		if err != nil {
			return nil, errors.Wrap(err, "failed to getEb")
		}
		switch modRM.rm {
		case 4:
			return mem8BaseDisp8{base: SI, disp8: disp8}, nil
		case 5:
			return mem8BaseDisp8{base: DI, disp8: disp8}, nil
		case 6:
			return mem8BaseDisp8{base: BP, disp8: disp8}, nil
		case 7:
			return mem8BaseDisp8{base: BX, disp8: disp8}, nil
		default:
			return nil, errors.Errorf("illegal or not yet implemeted for rm: %d", modRM.rm)
		}
	default:
		return nil, errors.Errorf("illegal or not yet implemented for mod: %d", modRM.mod)
	}
}

func decodeModRegRM(at *address, memory *memory) (byte, byte, byte, error) {
	buf, err := memory.readByte(at)
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "failed to parse byte")
	}

	mod := (buf & 0xc0) >> 6     // 0b11000000
	reg := (buf & 0x38) >> 3     // 0b00111000
	rm  := buf & 0x07 // 0b00000111

	return mod, reg, rm, nil
}

// assume that reader for load module is passed
// inst, read bytes, error
func decodeInst(reader io.Reader) (interface{}, int, *segmentOverride, error) {
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, 0, nil, err
	}
	memory := newMemory(bytes)
	address := newAddress(0, 0)
	return decodeInstWithMemory(address, memory)
}

// inst, read bytes, register overriding, error
func decodeInstWithMemory(initialAddress *address, memory *memory) (interface{}, int, *segmentOverride, error) {
	var inst interface{}
	currentAddress := initialAddress
	initialRealAddress := initialAddress.realAddress()

	rawOpcode, err := memory.readByte(currentAddress)
	if err != nil {
		return inst, -1, nil, errors.Wrap(err, "failed to parse opcode")
	}

	switch rawOpcode {
	// add r16,r/m16
	// 03 /r
	case 0x03:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x03")
		}
		dest, err := modRM.getGv()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x03")
		}
		src, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x03")
		}
		inst = instAdd{dest: dest, src: src}

	// push ds
	// 1e
	case 0x1e:
		inst = instPushSreg{src: DS}

	case 0x1f:
		inst = instPopSreg{dest: DS}

	// and r/m8,r8
	// 20 /r
	case 0x20:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x20")
		}
		dest, err := modRM.getEb(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x20")
		}
		src, err := modRM.getGb()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x20")
		}
		inst = instAnd{dest: dest, src: src}

	// segment override by ES
	case 0x26:
		inst, _, _, err := decodeInstWithMemory(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode")
		}
		return inst, currentAddress.realAddress() - initialRealAddress, &segmentOverride{sreg: ES}, nil

	// sub r8,r/m8
	// 2a /r
	case 0x2a:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x2a")
		}
		dest, err := modRM.getGb()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x2a")
		}
		src, err := modRM.getEb(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x2a")
		}
		inst = instSub{dest: dest, src: src}

	// sub r16,r/m16
	// 2b /r
	case 0x2b:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x2b")
		}
		dest, err := modRM.getGv()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x2b")
		}
		src, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x2b")
		}
		inst = instSub{dest: dest, src: src}

	// xor r16,r/m16
	// 33 /r
	case 0x33:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3b")
		}
		dest, err := modRM.getGv()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3b")
		}
		src, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3b")
		}
		inst = instXor{dest: dest, src: src}

	// cmp r16,r/m16
	// 3b /r
	case 0x3b:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3b")
		}
		dest, err := modRM.getGv()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3b")
		}
		src, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3b")
		}
		inst = instCmp{dest: dest, src: src}

	// cmp al,imm8
	// 3c ib
	case 0x3c:
		b, err := memory.readBytes(currentAddress, 1)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3c")
		}
		src, err := newImm8(bytes.NewReader(b))
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x3c")
		}
		inst = instCmp{dest: reg8{value: AL}, src: src}

	// inc ax
	case 0x40:
		inst = instInc{dest: AX}
	// inc cx
	case 0x41:
		inst = instInc{dest: CX}
	// inc dx
	case 0x42:
		inst = instInc{dest: DX}
	// inc bx
	case 0x43:
		inst = instInc{dest: BX}
	// inc sp
	case 0x44:
		inst = instInc{dest: SP}
	// inc bp
	case 0x45:
		inst = instInc{dest: BP}
	// inc si
	case 0x46:
		inst = instInc{dest: SI}
	// inc di
	case 0x47:
		inst = instInc{dest: DI}

	// dec ax
	case 0x48:
		inst = instDec{dest: AX}
	// dec cx
	case 0x49:
		inst = instDec{dest: CX}
	// dec dx
	case 0x4a:
		inst = instDec{dest: DX}
	// dec bx
	case 0x4b:
		inst = instDec{dest: BX}
	// dec sp
	case 0x4c:
		inst = instDec{dest: SP}
	// dec bp
	case 0x4d:
		inst = instDec{dest: BP}
	// dec si
	case 0x4e:
		inst = instDec{dest: SI}
	// dec di
	case 0x4f:
		inst = instDec{dest: DI}

	// push ax
	case 0x50:
		inst = instPush{src: AX}
	// push cx
	case 0x51:
		inst = instPush{src: CX}
	// push dx
	case 0x52:
		inst = instPush{src: DX}
	// push bx
	case 0x53:
		inst = instPush{src: BX}
	// push sp
	case 0x54:
		inst = instPush{src: SP}
	// push bp
	case 0x55:
		inst = instPush{src: BP}
	// push si
	case 0x56:
		inst = instPush{src: SI}
	// push di
	case 0x57:
		inst = instPush{src: DI}

	// pop ax
	case 0x58:
		inst = instPop{dest: AX}
	// pop cx
	case 0x59:
		inst = instPop{dest: CX}
	// pop dx
	case 0x5a:
		inst = instPop{dest: DX}
	// pop bx
	case 0x5b:
		inst = instPop{dest: BX}
	// pop sp
	case 0x5c:
		inst = instPop{dest: SP}
	// pop bp
	case 0x5d:
		inst = instPop{dest: BP}
	// pop si
	case 0x5e:
		inst = instPop{dest: SI}
	// pop di
	case 0x5f:
		inst = instPop{dest: DI}

	case 0x72:
		offset, err := memory.readInt8(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}
		inst = instJb{rel8: offset}

	case 0x73:
		imm8, err := memory.readInt8(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}
		inst = instJae{rel8: imm8}

	// je rel8
	// 74 cb
	case 0x74:
		imm8, err := memory.readInt8(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}
		inst = instJeRel8{rel8: imm8}

	// jne rel8
	// 75 cb
	case 0x75:
		imm8, err := memory.readInt8(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse imm8")
		}
		inst = instJneRel8{rel8: imm8}

	case 0x80:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x80")
		}
		dest, err := modRM.getEb(currentAddress, memory)
		if err != nil {
			return nil, -1, nil, errors.Errorf("failed to decode 0x80")
		}
		b, err := memory.readBytes(currentAddress, 1)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x80")
		}
		src, err := newImm8(bytes.NewReader(b))
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x80")
		}

		switch modRM.reg {
		// and r/m8, imm8
		case 4:
			inst = instAnd{dest: dest, src: src}

		// cmp r/m8,imm8
		// 80 /7 ib
		case 7:
			inst = instCmp{dest: dest, src: src}

		default:
			return nil, -1, nil, errors.Errorf("unknown reg: %d", modRM.reg)
		}

	case 0x81:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x81")
		}

		switch modRM.reg {
		case 5:
			// sub r/m16,imm16
			// 81 /5 iw
			dest, err := modRM.getEv(currentAddress, memory)
			if err != nil {
				return nil, -1, nil, errors.Errorf("failed to decode 0x81")
			}
			b, err := memory.readBytes(currentAddress, 2)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to decode 0x81")
			}
			src, err := newImm16(bytes.NewReader(b))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to decode 0x81")
			}
			inst = instSub{dest: dest, src: src}

		case 7:
			// cmp r/m16,imm16
			// 81 /7 iw
			dest, err := modRM.getEv(currentAddress, memory)
			if err != nil {
				return nil, -1, nil, errors.Errorf("failed to decode 0x81")
			}
			b, err := memory.readBytes(currentAddress, 2)
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to decode 0x81")
			}
			src, err := newImm16(bytes.NewReader(b))
			if err != nil {
				return inst, -1, nil, errors.Wrap(err, "failed to decode 0x81")
			}
			inst = instCmp{dest: dest, src: src}

		default:
			return inst, -1, nil, errors.Errorf("unknown reg value: %d", modRM.reg)
		}

	// add r/m16, imm8
	// 83 /5 -> sub r/m16, imm8
	// 83 /7 ib ->  cmp r/m16,imm8
	case 0x83:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x83")
		}
		dest, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return nil, -1, nil, errors.Errorf("failed to decode 0x83")
		}
		b, err := memory.readBytes(currentAddress, 1)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x83")
		}
		src, err := newImm8(bytes.NewReader(b))
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x83")
		}

		switch modRM.reg {
		// add
		case 0:
			inst = instAdd{dest: dest, src: src}

		// sub
		case 5:
			inst = instSub{dest: dest, src: src}

		// cmp
		case 7:
			inst = instCmp{dest: dest, src: src}

		default:
			return nil, -1, nil, errors.Errorf("expect reg is 0 but %d", modRM.reg)
		}

	// 88 /r
	// mov r/m8,r8
	case 0x88:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x88")
		}
		dest, err := modRM.getEb(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x88")
		}
		src, err := modRM.getGb()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x88")
		}
		inst = instMov{dest: dest, src: src}

	// 89 /r
	// mov r/m16,r16
	case 0x89:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x89")
		}
		dest, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x89")
		}
		src, err := modRM.getGv()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x89")
		}
		inst = instMov{dest: dest, src: src}

	// mov r8,r/m8
	// 8A /r
	case 0x8a:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8a")
		}
		dest, err := modRM.getGb()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8a")
		}
		src, err := modRM.getEb(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8a")
		}
		inst = instMov{dest: dest, src: src}

	// 8b /r (/r indicates that the ModR/M byte of the instruction contains a register operand and an r/m operand)
	// mov r16,r/m16
	case 0x8b:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8b")
		}
		dest, err := modRM.getGv()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8b")
		}
		src, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8b")
		}
		inst = instMov{dest: dest, src: src}

	// 8c /r
	// mov r/m16,Sreg
	case 0x8c:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8c")
		}
		dest, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8c")
		}
		src, err := modRM.getSw()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8c")
		}
		inst = instMov{dest: dest, src: src}

	// lea r16,m
	// 8d /r
	case 0x8d:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8d")
		}
		dest, err := modRM.getGv()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8d")
		}
		src, err := modRM.getM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8d")
		}
		inst = instLea{dest: dest, src: src}

	// 8e /r
	// mov Sreg,r/m16
	// Sreg ES=0, CS=1, SS=2, DS=3, FS=4, GS=5
	case 0x8e:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8e")
		}
		dest, err := modRM.getSw()
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8e")
		}
		src, err := modRM.getEw(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0x8e")
		}
		inst = instMov{dest: dest, src: src}

	// mov ax,moffs16
	// A1
	case 0xa1:
		imm, err := memory.readWord(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm16")
		}
		dest := reg16{value: AX}
		src := mem16Disp16{offset: imm}
		inst = instMov{dest: dest, src: src}

	// mov moffs8,al
	// A2
	case 0xa2:
		offset, err := memory.readWord(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm16")
		}
		dest := mem8Disp16{offset: offset}
		src := reg8{value: AL}
		inst = instMov{dest: dest, src: src}

	// mov moffs16,ax
	// A3
	case 0xa3:
		offset, err := memory.readWord(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm16")
		}
		dest := mem16Disp16{offset: offset}
		src := reg16{value: AX}
		inst = instMov{dest: dest, src: src}

	// stosb
	case 0xaa:
		inst = instStosb{}

	// b0+ rb ib
	// mov r8,imm8
	case 0xb0, 0xb1, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7:
		imm, err := memory.readBytes(currentAddress, 1)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		dest, err := newReg8(rawOpcode - 0xb0)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to create dest operand")
		}
		src, err := newImm8(bytes.NewReader(imm))
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to create src operand")
		}
		inst = instMov{dest: dest, src: src}

	// b8+ rw iw
	// mov r16,imm16
	case 0xb8, 0xb9, 0xba, 0xbb, 0xbc, 0xbd, 0xbe, 0xbf:
		// ax
		bs, err := memory.readBytes(currentAddress, 2)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode imm8")
		}
		dest, err := newReg16(rawOpcode - 0xb8)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "fialed to create dest operand")
		}
		src, err := newImm16(bytes.NewReader(bs))
		if err != nil {
			return inst, -1, nil, errors.Errorf("failed to create dest operand")
		}
		inst = instMov{dest: dest, src: src}

	// shl r/m16,imm8
	case 0xc1:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc1")
		}
		dest, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc1")
		}
		bs, err := memory.readBytes(currentAddress, 1)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc1")
		}
		src, err := newImm8(bytes.NewReader(bs))
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc1")
		}

		switch modRM.reg {
		case 4:
			inst = instShl{dest: dest, src: src}
		default:
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc1")
		}

	// ret (near return)
	case 0xc3:
		inst = instRet{}

	// mov r/m16,imm16
	// c7 /0 iw
	case 0xc7:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc7")
		}

		if modRM.reg != 0 {
			return inst, -1, nil, errors.Errorf("reg should be 0 but %d", modRM.reg)
		}

		dest, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc7")
		}
		bs, err := memory.readBytes(currentAddress, 2)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc7")
		}
		src, err := newImm16(bytes.NewReader(bs))
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xc7")
		}
		inst = instMov{dest: dest, src: src}

	// int imm8
	case 0xcd:
		operand, err := memory.readByte(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse operand")
		}
		inst = instInt{operand: operand}

	case 0xd1:
		modRM, err := newModRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xd1")
		}
		dest, err := modRM.getEv(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xd1")
		}
		src := imm8{value: 1}

		switch modRM.reg {
		// shl r/m16,1
		// d1 /4
		case 4:
			inst = instShl{dest: dest, src: src}

		// shr r/m16,1
		// d1 /4
		case 5:
			inst = instShr{dest: dest, src: src}

		default:
			return inst, -1, nil, errors.Wrap(err, "failed to decode 0xd1")
		}

	// call rel16
	case 0xe8:
		rel, err := memory.readInt16(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse int16")
		}
		inst = instCall{rel: rel}

	// jmp rel16
	case 0xe9:
		rel, err := memory.readInt16(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse int16")
		}
		inst = instJmpRel16{rel: rel}

	// jmp rel8
	case 0xeb:
		rel, err := memory.readInt8(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse int16")
		}
		inst = instJmpRel16{rel: int16(rel)}

	case 0xf3:
		stringOperation, err := memory.readByte(currentAddress)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to parse stringOperation")
		}
		switch stringOperation {
		case 0xa4:
			// rep movsb
			inst = instRepMovsb{}
		case 0xaa:
			// rep stosb
			inst = instRepStosb{}
		case 0xae:
			// repe scasb
			inst = instRepeScasb{}
		case 0xaf:
			// repe scasw
			inst = instRepeScasw{}
		default:
			return inst, -1, nil, errors.Errorf("not yet implemented string instruction")
		}

	// sti
	case 0xfb:
		inst = instSti{}

	// cld
	case 0xfc:
		inst = instCld{}

	case 0xff:
		mod, reg, rm, err := decodeModRegRM(currentAddress, memory)
		if err != nil {
			return inst, -1, nil, errors.Wrap(err, "failed to decode mod/reg/rm")
		}

		switch reg {
		case 2:
			if mod == 0 && rm == 6 {
				offset, err := memory.readWord(currentAddress)
				if err != nil {
					return inst, -1, nil, errors.Wrap(err, "failed to parse word")
				}
				inst = instCallAbsoluteIndirectMem16{offset: offset}
			} else {
				return inst, -1, nil, errors.Errorf("not yet implemented")
			}
		default:
			return inst, -1, nil, errors.Errorf("illegal or not implemented for reg: %d", reg)
		}


	default:
		return inst, -1, nil, errors.Errorf("unknown opcode: 0x%02x", rawOpcode)
	}
	return inst, currentAddress.realAddress() - initialRealAddress, nil, nil
}

// for int 21
type intHandler func(*state, *memory) error
type intHandlers map[uint8]intHandler

func intHandler30(s *state, memory *memory) error {
	// do nothing for now
	return nil
}

func intHandler4a(s *state, memory *memory) error {
	// do nothing for now
	return nil
}

func intHandler4c(s *state, memory *memory) error {
	s.exitCode = exitCode(s.al())
	s.shouldExit = true
	return nil
}

// DS:DX has the address of string
// string should be ended with '$'
func intHandler09(s *state, memory *memory) error {
	var bs []byte
	startAddress := newAddressFromWord(s.ds, s.dx)
	for {
		b, err := memory.readByte(startAddress)
		if err != nil {
			return err
		}
		if b == '$' {
			break
		}
		bs = append(bs, b)
	}
	fmt.Print(string(bs))
	return nil
}

// FIXME: Type general registers, segment registers respectively
type state struct {
	ax, cx, dx, bx, sp, bp, si, di, ss, cs, ip, ds, es word
	eflags dword
	exitCode                   exitCode
	shouldExit                 bool
	intHandlers                intHandlers
}

const (
	EFLAGS_ZF     = 0x00000040
	EFLAGS_ZF_INV = 0xffffffbf
	EFLAGS_CF     = 0x00000001
	EFLAGS_CF_INV = 0xfffffffe
	EFLAGS_DF     = 0x00000200
	EFLAGS_DF_INV = 0xfffffdff
)

func newState(header *header, customIntHandlers intHandlers) state {
	// --- Prepare interrupted handlers

	intHandlers := make(intHandlers)
	for k, v := range customIntHandlers {
		intHandlers[k] = v
	}

	// int 21 30h
	if _, ok := intHandlers[0x30]; !ok {
		intHandlers[0x30] = intHandler30
	}

	// int 21 4ah
	if _, ok := intHandlers[0x4a]; !ok {
		intHandlers[0x4a] = intHandler4a
	}

	// int 21 4ch
	if _, ok := intHandlers[0x4c]; !ok {
		intHandlers[0x4c] = intHandler4c
	}

	// int 21 09h
	if _, ok := intHandlers[0x09]; !ok {
		intHandlers[0x09] = intHandler09
	}

	return state{
		sp: header.exInitSP,
		ss: header.exInitSS,
		ip: header.exInitIP,
		cs: header.exInitCS,
		intHandlers: intHandlers}
}

func (s state) al() uint8 {
	return uint8(s.ax & 0x00ff)
}

func (s state) cl() uint8 {
	return uint8(s.cx & 0x00ff)
}

func (s state) dl() uint8 {
	return uint8(s.dx & 0x00ff)
}

func (s state) bl() uint8 {
	return uint8(s.bx & 0x00ff)
}

func (s state) ah() uint8 {
	return uint8(s.ax >> 8)
}

func (s state) ch() uint8 {
	return uint8(s.cx >> 8)
}

func (s state) dh() uint8 {
	return uint8(s.dx >> 8)
}

func (s state) bh() uint8 {
	return uint8(s.bx >> 8)
}

func (s state) addressIP() *address {
	return newAddressFromWord(s.cs, s.ip)
}

func (s state) addressSP() *address {
	return newAddressFromWord(s.ss, s.sp)
}

func (s state) addressFromBaseAndDisp(base registerW, disp int) (*address, error) {
	var vBase word
	var err error
	if vBase, err = s.readWordGeneralReg(base); err != nil {
		return nil, errors.Wrap(err, "failed to get address from base and disp")
	}

	var address *address
	if base == BP {
		address = newAddressFromWord(s.ss, vBase)
	} else {
		address = newAddressFromWord(s.ds, vBase)
	}
	address.plus(disp)
	return address, nil
}

// return true if zf == 1
func (s state) isActiveZF() bool {
	zf := s.eflags & EFLAGS_ZF
	return zf != 0
}

// return true if zf == 0
func (s state) isNotActiveZF() bool {
	return !s.isActiveZF()
}

func (s state) setZF() state {
	s.eflags = s.eflags | EFLAGS_ZF
	return s
}

func (s state) resetZF() state {
	s.eflags = s.eflags & EFLAGS_ZF_INV
	return s
}

// return true if cf == 1
func (s state) isActiveCF() bool {
	cf := s.eflags & EFLAGS_CF
	return cf != 0
}

// return true if cf == 0
func (s state) isNotActiveCF() bool {
	return !s.isActiveCF()
}

func (s state) setCF() state {
	s.eflags = s.eflags | EFLAGS_CF
	return s
}

func (s state) resetCF() state {
	s.eflags = s.eflags & EFLAGS_CF_INV
	return s
}

// return true if df == 1
func (s state) isActiveDF() bool {
	df := s.eflags & EFLAGS_DF
	return df != 0
}

// return true if df == 0
func (s state) isNotActiveDF() bool {
	return !s.isActiveDF()
}

func (s state) setDF() state {
	s.eflags = s.eflags | EFLAGS_DF
	return s
}

func (s state) resetDF() state {
	s.eflags = s.eflags & EFLAGS_DF_INV
	return s
}

func (s state) readWordGeneralReg(r registerW) (word, error) {
	switch r {
	case AX:
		return s.ax, nil
	case CX:
		return s.cx, nil
	case DX:
		return s.dx, nil
	case BX:
		return s.bx, nil
	case SP:
		return s.sp, nil
	case BP:
		return s.bp, nil
	case SI:
		return s.si, nil
	case DI:
		return s.di, nil
	default:
		return 0, errors.Errorf("illegal registerW or not implemented: %d", r)
	}
}

func (s state) readByteGeneralReg(r registerB) (uint8, error) {
	switch r {
	case AL:
		return s.al(), nil
	case CL:
		return s.cl(), nil
	case DL:
		return s.dl(), nil
	case BL:
		return s.bl(), nil
	case AH:
		return s.ah(), nil
	case CH:
		return s.ch(), nil
	case DH:
		return s.dh(), nil
	case BH:
		return s.bh(), nil
	default:
		return 0, errors.Errorf("illegal registerB or not implemented: %d", r)
	}
}

func (s state) readWordSreg(r registerS) (word, error) {
	switch r {
	case ES:
		return s.es, nil
	case CS:
		return s.cs, nil
	case SS:
		return s.ss, nil
	case DS:
		return s.ds, nil
		/*
	case FS:
		return s.fs, nil
	case GS:
		return s.gs, nil
		*/
	default:
		return 0, errors.Errorf("illegal number for registerS:%d", r)
	}
}

func (s state) writeByteGeneralReg(r registerB, b uint8) (state, error) {
	switch r {
	case AL:
		s.ax = (s.ax & 0xff00) | word(b)
	case CL:
		s.cx = (s.cx & 0xff00) | word(b)
	case DL:
		s.dx = (s.dx & 0xff00) | word(b)
	case BL:
		s.bx = (s.bx & 0xff00) | word(b)
	case AH:
		s.ax = (s.ax & 0x00ff) | (word(b) << 8)
	case CH:
		s.cx = (s.cx & 0x00ff) | (word(b) << 8)
	case DH:
		s.dx = (s.dx & 0x00ff) | (word(b) << 8)
	case BH:
		s.bx = (s.bx & 0x00ff) | (word(b) << 8)
	default:
		return s, errors.Errorf("illegal number for registerB: %d", r)
	}
	return s, nil
}

func (s state) writeWordGeneralReg(r registerW, w word) (state, error) {
	switch r {
	case AX:
		s.ax = w
		return s, nil
	case CX:
		s.cx = w
		return s, nil
	case DX:
		s.dx = w
		return s, nil
	case BX:
		s.bx = w
		return s, nil
	case SP:
		s.sp = w
		return s, nil
	case BP:
		s.bp = w
		return s, nil
	case SI:
		s.si = w
		return s, nil
	case DI:
		s.di = w
		return s, nil
	default:
		return s, errors.Errorf("illegal registerW or not implemented: %d", r)
	}
}

func (s state) writeWordSreg(r registerS, w word) (state, error) {
	switch r {
	case ES:
		s.es = w
		return s, nil
	case CS:
		s.cs = w
		return s, nil
	case SS:
		s.ss = w
		return s, nil
	case DS:
		s.ds = w
		return s, nil
		/*
	case FS:
		return s.fs, nil
	case GS:
		return s.gs, nil
		*/
	default:
		return s, errors.Errorf("illegal number for registerS:%d", r)
	}
}

func (s state) pushWord(w word, memory *memory) (state, error) {
	s.sp -= 2
	err := memory.writeWord(s.addressSP(), w)
	if err != nil {
		return s, errors.Wrap(err, "failed to push word")
	}
	return s, nil
}

func (s state) popWord(memory *memory) (word, state, error) {
	w, err := memory.readWord(s.addressSP())
	if err != nil {
		return 0, s, errors.Wrap(err, "failed in execPop")
	}
	s.sp += 2
	return w, s, nil
}

// --- execute instruction

func execMov(inst instMov, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	var v int
	var err error

	// FIXME
	initDS := state.ds
	if segmentOverride != nil {
		switch segmentOverride.sreg {
		case ES:
			state.ds = state.es
		default:
			return state, errors.Errorf("not yet implemented or illegal sreg: %#v", segmentOverride.sreg)
		}
	}

	if v, err = inst.src.read(state, memory); err != nil {
		state.ds = initDS
		return state, err
	}

	state, err = inst.dest.write(v, state, memory)
	if segmentOverride != nil {
		state.ds = initDS
	}
	return state, err
}

func execShl(inst instShl, state state, memory *memory) (state, error) {
	var l, r int
	var err error

	if r, err = inst.src.read(state, memory); err != nil {
		return state, err
	}
	if l, err = inst.dest.read(state, memory); err != nil {
		return state, err
	}

	state, err = inst.dest.write(l << uint(r), state, memory)
	return state, err
}

func execShr(inst instShr, state state, memory *memory) (state, error) {
	var l, r int
	var err error

	if r, err = inst.src.read(state, memory); err != nil {
		return state, err
	}
	if l, err = inst.dest.read(state, memory); err != nil {
		return state, err
	}

	state, err = inst.dest.write(l >> uint(r), state, memory)
	return state, err
}

func execSub(inst instSub, state state, memory *memory) (state, error) {
	var l, r int
	var err error
	if r, err = inst.src.read(state, memory); err != nil {
		return state, err
	}
	if l, err = inst.dest.read(state, memory); err != nil {
		return state, err
	}
	state, err = inst.dest.write(l-r, state, memory)
	return state, err
}

func execLea(inst instLea, state state, memory *memory) (state, error) {
	var address *address
	var err error
	if address, err = inst.src.address(state); err != nil {
		return state, err
	}
	state, err = inst.dest.write(int(address.offset), state, memory)
	return state, err
}

func execInt(inst instInt, state state, memory *memory) (state, error) {
	switch inst.operand {
	case 0x21:
		if handler, ok := state.intHandlers[state.ah()]; ok {
			err := handler(&state, memory)
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

func execPush(inst instPush, state state, memory *memory) (state, error) {
	v, err := state.readWordGeneralReg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPush")
	}
	state, err = state.pushWord(v, memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPush")
	}
	return state, nil
}

func execPushSreg(inst instPushSreg, state state, memory *memory) (state, error) {
	v, err := state.readWordSreg(inst.src)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPushSreg")
	}
	state, err = state.pushWord(v, memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPushSreg")
	}
	return state, nil
}

func execPop(inst instPop, state state, memory *memory) (state, error) {
	w, state, err := state.popWord(memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPop")
	}
	state, err = state.writeWordGeneralReg(inst.dest, w)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPop")
	}
	return state, nil
}

func execPopSreg(inst instPopSreg, state state, memory *memory) (state, error) {
	w, state, err := state.popWord(memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPopSreg")
	}
	state, err = state.writeWordSreg(inst.dest, w)
	if err != nil {
		return state, errors.Wrap(err, "failed in execPopSreg")
	}
	return state, nil
}

func execCall(inst instCall, state state, memory *memory) (state, error) {
	state, err := state.pushWord(state.ip, memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execCall")
	}
	state.ip = word(int16(state.ip) + inst.rel)
	return state, nil
}

func execCallAbsoluteIndirectMem16(inst instCallAbsoluteIndirectMem16, state state, memory *memory) (state, error) {
	state, err := state.pushWord(state.ip, memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execCallAbsoluteIndirectMem16")
	}
	address := newAddressFromWord(state.ds, inst.offset)
	callOffset, err := memory.readWord(address)
	if err != nil {
		return state, errors.Wrap(err, "failed in execCallAbsoluteIndirectMem16")
	}
	state.ip = callOffset
	return state, nil
}

func execRet(inst instRet, state state, memory *memory) (state, error) {
	returnAddress, state, err := state.popWord(memory)
	if err != nil {
		return state, errors.Wrap(err, "failed in execRet")
	}
	state.ip = returnAddress
	return state, nil
}

func execJmpRel16(inst instJmpRel16, state state, memory *memory) (state, error) {
	state.ip = word(int16(state.ip) + inst.rel)
	return state, nil
}

func execSti(inst instSti, state state, memory *memory) (state, error) {
	// do nothing now
	return state, nil
}

func execAnd(inst instAnd, state state, memory *memory) (state, error) {
	var l, r int
	var err error
	if r, err = inst.src.read(state, memory); err != nil {
		return state, err
	}
	if l, err = inst.dest.read(state, memory); err != nil {
		return state, err
	}
	state, err = inst.dest.write(l & r, state, memory)
	return state, err
}

func execAdd(inst instAdd, state state, memory *memory) (state, error) {
	var l, r int
	var err error

	if r, err = inst.src.read(state, memory); err != nil {
		return state, err
	}
	if l, err = inst.dest.read(state, memory); err != nil {
		return state, err
	}

	state, err = inst.dest.write(l+r, state, memory)
	return state, err
}

func execCmp(inst instCmp, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	var l, r int
	var err error

	// FIXME
	initDS := state.ds
	if segmentOverride != nil {
		switch segmentOverride.sreg {
		case ES:
			state.ds = state.es
		default:
			return state, errors.Errorf("not yet implemented or illegal sreg: %#v", segmentOverride.sreg)
		}
	}

	if r, err = inst.src.read(state, memory); err != nil {
		state.ds = initDS
		return state, err
	}
	if l, err = inst.dest.read(state, memory); err != nil {
		state.ds = initDS
		return state, err
	}
	if l == r {
		state = state.setZF()
		state = state.resetCF()
	} else if l < r {
		state = state.resetZF()
		state = state.setCF()
	} else {
		state = state.resetZF()
		state = state.resetCF()
	}

	if segmentOverride != nil {
		state.ds = initDS
	}
	return state, err
}

func execInstJneRel8(inst instJneRel8, state state) (state, error) {
	if state.isNotActiveZF() {
		state.ip = word(int16(state.ip) + int16(inst.rel8))
	}
	return state, nil
}

func execInstJb(inst instJb, state state) (state, error) {
	if state.isActiveCF() {
		state.ip = word(int16(state.ip) + int16(inst.rel8))
	}
	return state, nil
}

func execInstCld(inst instCld, state state) (state, error) {
	state = state.resetDF()
	return state, nil
}

func execScasb(state state, memory *memory) (state, error) {
	vAL, err := state.readByteGeneralReg(AL)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vSeg, err := state.readWordSreg(ES) // use ES for DI in string instructions
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vDI, err := state.readWordGeneralReg(DI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	address := newAddressFromWord(vSeg, vDI)
	vMem, err := memory.readByte(address)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	if vAL == vMem {
		state = state.setZF()
	} else {
		state = state.resetZF()
	}
	if state.isNotActiveDF() {
		state, err = state.writeWordGeneralReg(DI, vDI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	} else {
		state, err = state.writeWordGeneralReg(DI, vDI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	}
	return state, nil
}

func execScasw(state state, memory *memory) (state, error) {
	vAX, err := state.readWordGeneralReg(AX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vSeg, err := state.readWordSreg(ES) // use ES for DI in string instructions
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vDI, err := state.readWordGeneralReg(DI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	address := newAddressFromWord(vSeg, vDI)
	vMem, err := memory.readWord(address)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	if vAX == vMem {
		state = state.setZF()
	} else {
		state = state.resetZF()
	}
	if state.isNotActiveDF() {
		state, err = state.writeWordGeneralReg(DI, vDI + 2)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	} else {
		state, err = state.writeWordGeneralReg(DI, vDI - 2)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	}
	return state, nil
}

func execMovsb(state state, memory *memory) (state, error) {
	vDS, err := state.readWordSreg(DS) // use DS for SI in string instructions
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vES, err := state.readWordSreg(ES) // use ES for DI in string instructions
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vSI, err := state.readWordGeneralReg(SI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vDI, err := state.readWordGeneralReg(DI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	vMem, err := memory.readByte(newAddressFromWord(vDS, vSI))
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	err = memory.writeByte(newAddressFromWord(vES, vDI), vMem)
	if err != nil {
		return state, errors.Wrap(err, "failed in execScasb")
	}
	if state.isNotActiveDF() {
		state, err = state.writeWordGeneralReg(SI, vSI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
		state, err = state.writeWordGeneralReg(DI, vDI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	} else {
		state, err = state.writeWordGeneralReg(SI, vSI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
		state, err = state.writeWordGeneralReg(DI, vDI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execScasb")
		}
	}
	return state, nil
}

func execStosb(state state, memory *memory) (state, error) {
	vES, err := state.readWordSreg(ES)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	vDI, err := state.readWordGeneralReg(DI)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	vAL, err := state.readByteGeneralReg(AL)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	err = memory.writeByte(newAddressFromWord(vES, vDI), vAL)
	if err != nil {
		return state, errors.Wrap(err, "failed in execStosb")
	}
	if state.isNotActiveDF() {
		state, err = state.writeWordGeneralReg(DI, vDI + 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execStosb")
		}
	} else {
		state, err = state.writeWordGeneralReg(DI, vDI - 1)
		if err != nil {
			return state, errors.Wrap(err, "failed in execStosb")
		}
	}
	return state, nil
}

func execInstRepeScasb(inst instRepeScasb, state state, memory *memory) (state, error) {
	count, err := state.readWordGeneralReg(CX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	for count > 0 && state.isActiveZF() {
		state, err = execScasb(state, memory)
		if err != nil {
			return state, errors.Wrap(err, "failed in execInstRepeScasb")
		}
		count--
	}
	state, err = state.writeWordGeneralReg(CX, count)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	return state, nil
}

func execInstRepeScasw(inst instRepeScasw, state state, memory *memory) (state, error) {
	count, err := state.readWordGeneralReg(CX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasw")
	}
	for count > 0 && state.isActiveZF() {
		state, err = execScasw(state, memory)
		if err != nil {
			return state, errors.Wrap(err, "failed in execInstRepeScasw")
		}
		count--
	}
	state, err = state.writeWordGeneralReg(CX, count)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasw")
	}
	return state, nil
}

func execInstRepMovsb(inst instRepMovsb, state state, memory *memory) (state, error) {
	count, err := state.readWordGeneralReg(CX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	for count > 0 {
		state, err = execMovsb(state, memory)
		if err != nil {
			return state, errors.Wrap(err, "failed in execInstRepeScasb")
		}
		count--
	}
	state, err = state.writeWordGeneralReg(CX, count)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	return state, nil
}

func execInstRepStosb(inst instRepStosb, state state, memory *memory) (state, error) {
	count, err := state.readWordGeneralReg(CX)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	for count > 0 {
		state, err = execStosb(state, memory)
		if err != nil {
			return state, errors.Wrap(err, "failed in execInstRepeScasb")
		}
		count--
	}
	state, err = state.writeWordGeneralReg(CX, count)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstRepeScasb")
	}
	return state, nil
}

func execInstStosb(inst instStosb, state state, memory *memory) (state, error) {
	return execStosb(state, memory)
}

func execInstJeRel8(inst instJeRel8, state state) (state, error) {
	if state.isActiveZF() {
		state.ip = word(int16(state.ip) + int16(inst.rel8))
	}
	return state, nil
}

func execInstInc(inst instInc, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	state, err = state.writeWordGeneralReg(inst.dest, v + 1)
	// TODO: Set ZF (so it is necessary to handle overflow...)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	return state, nil
}

func execInstDec(inst instDec, state state) (state, error) {
	v, err := state.readWordGeneralReg(inst.dest)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	state, err = state.writeWordGeneralReg(inst.dest, v - 1)
	// TODO: Set ZF (so it is necessary to handle overflow...)
	if err != nil {
		return state, errors.Wrap(err, "failed in execInstInc")
	}
	return state, nil
}

func execXor(inst instXor, state state, memory *memory) (state, error) {
	var l, r int
	var err error

	if r, err = inst.src.read(state, memory); err != nil {
		return state, err
	}
	if l, err = inst.dest.read(state, memory); err != nil {
		return state, err
	}

	state, err = inst.dest.write(l ^ r, state, memory)
	return state, err
}

func execInstJae(inst instJae, state state) (state, error) {
	if state.isNotActiveCF() {
		state.ip = word(int16(state.ip) + int16(inst.rel8))
	}
	return state, nil
}

func execute(shouldBeInst interface{}, state state, memory *memory, segmentOverride *segmentOverride) (state, error) {
	switch inst := shouldBeInst.(type) {
	case instMov:
		return execMov(inst, state, memory, segmentOverride)
	case instShl:
		return execShl(inst, state, memory)
	case instShr:
		return execShr(inst, state, memory)
	case instAdd:
		return execAdd(inst, state, memory)
	case instSub:
		return execSub(inst, state, memory)
	case instLea:
		return execLea(inst, state, memory)
	case instInt:
		return execInt(inst, state, memory)
	case instPush:
		return execPush(inst, state, memory)
	case instPushSreg:
		return execPushSreg(inst, state, memory)
	case instPop:
		return execPop(inst, state, memory)
	case instPopSreg:
		return execPopSreg(inst, state, memory)
	case instCall:
		return execCall(inst, state, memory)
	case instCallAbsoluteIndirectMem16:
		return execCallAbsoluteIndirectMem16(inst, state, memory)
	case instRet:
		return execRet(inst, state, memory)
	case instJmpRel16:
		return execJmpRel16(inst, state, memory)
	case instSti:
		return execSti(inst, state, memory)
	case instAnd:
		return execAnd(inst, state, memory)
	case instCmp:
		return execCmp(inst, state, memory, segmentOverride)
	case instJneRel8:
		return execInstJneRel8(inst, state)
	case instJb:
		return execInstJb(inst, state)
	case instCld:
		return execInstCld(inst, state)
	case instRepeScasb:
		return execInstRepeScasb(inst, state, memory)
	case instRepeScasw:
		return execInstRepeScasw(inst, state, memory)
	case instRepMovsb:
		return execInstRepMovsb(inst, state, memory)
	case instRepStosb:
		return execInstRepStosb(inst, state, memory)
	case instJeRel8:
		return execInstJeRel8(inst, state)
	case instInc:
		return execInstInc(inst, state)
	case instStosb:
		return execInstStosb(inst, state, memory)
	case instDec:
		return execInstDec(inst, state)
	case instXor:
		return execXor(inst, state, memory)
	case instJae:
		return execInstJae(inst, state)
	default:
		return state, errors.Errorf("unknown inst: %T", shouldBeInst)
	}
}

func runExeWithCustomIntHandlers(reader io.Reader, intHandlers intHandlers) (state, error) {
	parser := newParser(reader)
	header, loadModule, err := parseHeaderWithParser(parser)
	if err != nil {
		return state{}, errors.Wrap(err, "error to parse header")
	}

	memory := newMemoryFromHeader(loadModule, header)

	s := newState(header, intHandlers)

	for {
		inst, readBytesCount, segmentOverride, err := decodeInstWithMemory(s.addressIP(), memory)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			} else {
				return state{}, errors.Wrap(err, "error to decode inst")
			}
		}
		debug.printf("decode inst %#v at 0x%04x:0x%04x\n", inst, s.cs, s.ip)

		s.ip = s.ip + word(readBytesCount)
		s, err = execute(inst, s, memory, segmentOverride)
		if err != nil {
			return state{}, errors.Wrap(err, "errors to execute")
		}
		if s.shouldExit {
			break
		}
		// x, _ := s.readWordGeneralReg(DX)
		// debug.printf("0x%04x\n", x)
		// debug.printf("0x%08x\n", s.eflags)
		// y, _ := s.readWordSreg(DS)
		// debug.printf("0x%04x\n", y)
		// z, _ := memory.readWord(s.realAddress(s.ds, x - 2))
		// debug.printf("0x%04x\n", z)
	}

	return s, nil
}

// (exit code, state, error)
func RunExe(reader io.Reader) (uint8, state, error) {
	state, err := runExeWithCustomIntHandlers(reader, make(intHandlers))
	return uint8(state.exitCode), state, err
}
