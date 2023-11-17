package infinity

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/npat-efault/crc16"
)

const (
	Ack02           = uint8(0x02)
	Ack06           = uint8(0x06) //opRESPONSE
	ReadTableBlock  = uint8(0x0b) //opREAD
	WriteTableBlock = uint8(0x0c) //opWRITE
	ChangeTableName = uint8(0x10)
	Nack            = uint8(0x15) //opERROR
	AlarmPacket     = uint8(0x1e)
	ReadObjectData  = uint8(0x22)
	ReadVariable    = uint8(0x62)
	WriteVariable   = uint8(0x63)
	AutoVariable    = uint8(0x64)
	ReadList        = uint8(0x75)
)

var crcConfig = &crc16.Conf{
	Poly: 0x8005, BitRev: true,
	IniVal: 0x0, FinVal: 0x0,
	BigEnd: false,
}

type Frame struct {
	dst     uint16
	src     uint16
	dataLen uint8
	op      uint8
	data    []byte
}

var writeAck = Frame{
	src:  DevSAM,
	dst:  DevTSTAT,
	op:   Ack06,
	data: []byte{0x00},
}

func checksum(b []byte) []byte {
	s := crc16.New(crcConfig)
	s.Write(b)
	return s.Sum(nil)
}

func (f Frame) String() string {
	return fmt.Sprintf("%x -> %x: %-8s %x", f.src, f.dst, opToString(f.op), f.data)
}

func (f Frame) Clone() Frame {
	f.data = append([]byte{}, f.data...)
	return f
}

type framePredicate func(Frame) bool
type frameHandler func(Frame)

func sourceRange(srcMin uint16, srcMax uint16) framePredicate {
	return func(f Frame) bool {
		return f.src >= srcMin && f.src <= srcMax
	}
}

var opsToString = [256]string{
	Ack02:           "ACK02",
	Ack06:           "ACK06",
	ReadTableBlock:  "READ",
	WriteTableBlock: "WRITE",
	ChangeTableName: "CHGTBN",
	Nack:            "NACK",
	AlarmPacket:     "ALARM",
	ReadObjectData:  "OBJRD",
	ReadVariable:    "RDVAR",
	WriteVariable:   "FORCE",
	AutoVariable:    "AUTO",
	ReadList:        "LIST",
}

func opToString(op uint8) string {
	if s := opsToString[op]; s != "" {
		return s
	} else {
		return fmt.Sprintf("UNKNOWN(%x)", op)
	}
}

func (frame *Frame) encode() []byte {
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

func (f *Frame) decode(buf []byte) bool {
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
	f.data = append([]byte{}, buf[8:l]...)

	return true
}
