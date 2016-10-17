package main

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/npat-efault/crc16"
)

const (
	opRESPONSE = uint8(0x06)
	opREAD     = uint8(0x0b)
	opWRITE    = uint8(0x0c)
	opERROR    = uint8(0x15)
)

var crcConfig = &crc16.Conf{
	Poly: 0x8005, BitRev: true,
	IniVal: 0x0, FinVal: 0x0,
	BigEnd: false,
}

type InfinityFrame struct {
	dst     uint16
	src     uint16
	dataLen uint8
	op      uint8
	data    []byte
}

var writeAck = &InfinityFrame{
	src:  devSAM,
	dst:  devTSTAT,
	op:   opRESPONSE,
	data: []byte{0x00},
}

func checksum(b []byte) []byte {
	s := crc16.New(crcConfig)
	s.Write(b)
	return s.Sum(nil)
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
	cksum := checksum(b.Bytes())
	b.Write(cksum)

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

	cksum := checksum(buf[:l])
	if !bytes.Equal(cksum, buf[l:]) {
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
