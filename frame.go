package main

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/jiguorui/crc16"
)

const (
	opRESPONSE = uint8(0x06)
	opREAD     = uint8(0x0b)
	opWRITE    = uint8(0x0c)
	opERROR    = uint8(0x15)
)

type InfinityFrame struct {
	dst     uint16
	src     uint16
	dataLen uint8
	op      uint8
	data    []byte
	cksum   uint16
}

var writeAck = &InfinityFrame{
	src:  devSAM,
	dst:  devTSTAT,
	op:   opRESPONSE,
	data: []byte{0x00},
}

func (f *InfinityFrame) String() string {
	return fmt.Sprintf("%x -> %x: %-8s %x", f.src, f.dst, f.opString(), f.data)
}

func (f *InfinityFrame) opString() string {
	switch f.op {
	case opRESPONSE:
		return "RESPONSE"
	case opREAD:
		return "READ"
	case opWRITE:
		return "WRITE"
	case opERROR:
		return "ERROR"
	default:
		return fmt.Sprintf("UNKNOWN(%x)", f.op)
	}
}

func (frame *InfinityFrame) encode() []byte {
	// b := make([]byte, 10 + len(frame.data))
	if len(frame.data) > 255 {
		panic("frame data too large")
	}

	var b bytes.Buffer

	binary.Write(&b, binary.BigEndian, frame.dst)
	binary.Write(&b, binary.BigEndian, frame.src)
	b.WriteByte(byte(len(frame.data)))
	b.WriteByte(0)
	b.WriteByte(0)
	b.WriteByte(frame.op)
	b.Write(frame.data)
	cksum := crc16.CheckSum(b.Bytes())
	var cksumbuf [2]byte
	binary.LittleEndian.PutUint16(cksumbuf[:], cksum)
	b.Write(cksumbuf[:])

	return b.Bytes()
}

func (f *InfinityFrame) decode(buf []byte) bool {
	nonzero := false
	for _, c := range buf {
		if c != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		return false
	}

	l := len(buf) - 2

	f.cksum = binary.LittleEndian.Uint16(buf[l:])

	cksum := crc16.CheckSum(buf[:l])
	if f.cksum != cksum {
		return false
	}

	f.dst = binary.BigEndian.Uint16(buf[0:2])
	f.src = binary.BigEndian.Uint16(buf[2:4])
	f.dataLen = buf[4]
	// Not sure what bytes 5 and 6 are
	f.op = buf[7]
	f.data = buf[8:l]

	return true
}
