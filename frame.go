package main

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/npat-efault/crc16"
)

const (
	ACK02             = uint8(0x02)
	ACK06             = uint8(0x06)
	READ_TABLE_BLOCK  = uint8(0x0b)
	WRITE_TABLE_BLOCK = uint8(0x0c)
	CHANGE_TABLE_NAME = uint8(0x10)
	NACK              = uint8(0x15)
	ALARM_PACKET      = uint8(0x1e)
	READ_OBJECT_DATA  = uint8(0x22)
	READ_VARIABLE     = uint8(0x62)
	WRITE_VARIABLE    = uint8(0x63)
	AUTO_VARIABLE     = uint8(0x64)
	READ_LIST         = uint8(0x75)
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
	op:   ACK06,
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
	case ACK02:
		return "ACK02"
	case ACK06:
		return "ACK06"
	case READ_TABLE_BLOCK:
		return "READ"
	case WRITE_TABLE_BLOCK:
		return "WRITE"
	case CHANGE_TABLE_NAME:
		return "CHGTBN"
	case NACK:
		return "NACK"
	case ALARM_PACKET:
		return "ALARM"
	case READ_OBJECT_DATA:
		return "OBJRD"
	case READ_VARIABLE:
		return "RDVAR"
	case WRITE_VARIABLE:
		return "FORCE"
	case AUTO_VARIABLE:
		return "AUTO"
	case READ_LIST:
		return "LIST"
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
