package main

import (
	"bytes"
	"encoding/binary"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tarm/serial"
)

const (
	devTSTAT = uint16(0x2001)
	devSAM   = uint16(0x9201)
)

const responseTimeout = 200
const responseRetries = 5

type snoopCallback func(*InfinityFrame)

type InfinityProtocolRawRequest struct {
	data *[]byte
}

type InfinityProtocolSnoop struct {
	srcMin uint16
	srcMax uint16
	cb     snoopCallback
}

type InfinityProtocol struct {
	device     string
	port       *serial.Port
	responseCh chan *InfinityFrame
	actionCh   chan *Action
	snoops     []InfinityProtocolSnoop
}

type Action struct {
	requestFrame  *InfinityFrame
	responseFrame *InfinityFrame
	ok            bool
	ch            chan bool
}

var readTimeout = time.Second * 5

func (p *InfinityProtocol) openSerial() error {
	log.Printf("opening serial interface: %s", p.device)
	if p.port != nil {
		p.port.Close()
	}

	c := &serial.Config{Name: p.device, Baud: 38400, ReadTimeout: readTimeout}
	var err error
	p.port, err = serial.OpenPort(c)
	if err != nil {
		return err
	}

	return nil
}

func (p *InfinityProtocol) Open() error {
	err := p.openSerial()
	if err != nil {
		return err
	}

	p.responseCh = make(chan *InfinityFrame, 32)
	p.actionCh = make(chan *Action)

	go p.reader()
	go p.broker()

	return nil
}

func (p *InfinityProtocol) handleFrame(frame *InfinityFrame) *InfinityFrame {
	log.Printf("read frame: %s", frame)

	switch frame.op {
	case opRESPONSE:
		if frame.dst == devSAM {
			p.responseCh <- frame
		}

		if len(frame.data) > 3 {
			for _, s := range p.snoops {
				if frame.src >= s.srcMin && frame.src <= s.srcMax {
					s.cb(frame)
				}
			}
		}
	case opWRITE:
		if frame.src == devTSTAT && frame.dst == devSAM {
			return writeAck
		}
	}

	return nil
}

func (p *InfinityProtocol) reader() {
	defer panic("exiting InfinityProtocol reader, this should never happen")

	msg := []byte{}
	buf := make([]byte, 1024)

	for {
		if p.port == nil {
			msg = []byte{}
			p.openSerial()
		}

		n, err := p.port.Read(buf)
		if n == 0 || err != nil {
			log.Printf("error reading from serial port: %s", err.Error())
			if p.port != nil {
				p.port.Close()
			}
			p.port = nil
			continue
		}
		// log.Printf("%q", buf[:n])
		msg = append(msg, buf[:n]...)
		// log.Printf("buf len is: %v", len(msg))

		for {
			if len(msg) < 10 {
				break
			}
			l := int(msg[4]) + 10
			if len(msg) < l {
				break
			}
			buf := msg[:l]

			frame := &InfinityFrame{}
			if frame.decode(buf) {
				response := p.handleFrame(frame)
				if response != nil {
					p.sendFrame(response.encode())
				}
				// Intentionally didn't do msg = msg[l:] to avoid potential
				// memory leak.  Not sure if it makes a difference...
				msg = msg[:copy(msg, msg[l:])]
			} else {
				// Corrupt message, move ahead one byte and continue parsing
				msg = msg[:copy(msg, msg[1:])]
			}
		}
	}
}

func (p *InfinityProtocol) broker() {
	defer panic("exiting InfinityProtocol broker, this should never happen")

	for {
		// log.Debug("entering action select")
		select {
		case action := <-p.actionCh:
			p.performAction(action)
		case <-p.responseCh:
			log.Warn("dropping unexpected response")
		}
	}
}

func (p *InfinityProtocol) performAction(action *Action) {
	log.Infof("encoded frame: %s", action.requestFrame)
	encodedFrame := action.requestFrame.encode()

	p.sendFrame(encodedFrame)
	ticker := time.NewTicker(time.Millisecond * responseTimeout)
	defer ticker.Stop()
	for tries := 0; tries < responseRetries; {
		select {
		case res := <-p.responseCh:
			if res.src != action.requestFrame.dst {
				continue
			}

			reqTable := action.requestFrame.data[0:3]
			resTable := res.data[0:3]

			if action.requestFrame.op == opREAD && !bytes.Equal(reqTable, resTable) {
				log.Printf("got response for incorrect table, is: %x expected: %x", resTable, reqTable)
				continue
			}
			action.responseFrame = res
			// log.Printf("got response!")
			action.ok = true
			action.ch <- true
			// log.Printf("sent action!")
			return
		case <-ticker.C:
			log.Debug("timeout waiting for response, retransmitting frame")
			p.sendFrame(encodedFrame)
			tries++
		}
	}

	log.Printf("action timed out")
	action.ch <- false
}

func (p *InfinityProtocol) send(dst uint16, op uint8, requestData []byte, response interface{}) bool {
	f := InfinityFrame{src: devSAM, dst: dst, op: op, data: requestData}
	act := &Action{requestFrame: &f, ch: make(chan bool)}

	// Send action to action handling goroutine
	p.actionCh <- act
	// Wait for response
	ok := <-act.ch

	if ok && op == opREAD && act.responseFrame != nil && act.responseFrame.data != nil && len(act.responseFrame.data) > 6 {
		raw, ok := response.(InfinityProtocolRawRequest)
		if ok {
			log.Printf(">>>> handling a RawRequest")
			*raw.data = append(*raw.data, act.responseFrame.data[6:]...)
			log.Printf("raw data length is: %d", len(*raw.data))
		} else {
			r := bytes.NewReader(act.responseFrame.data[6:])
			binary.Read(r, binary.BigEndian, response)
		}
		// log.Printf("%+v", data)
	}

	return ok
}

func (p *InfinityProtocol) Write(dst uint16, table []byte, addr []byte, params interface{}) bool {
	buf := new(bytes.Buffer)
	buf.Write(table[:])
	buf.Write(addr[:])
	binary.Write(buf, binary.BigEndian, params)

	return p.send(dst, opWRITE, buf.Bytes(), nil)
}

func (p *InfinityProtocol) WriteTable(dst uint16, table InfinityTable, flags uint8) bool {
	addr := table.addr()
	fl := []byte{0x00, 0x00, flags}
	return p.Write(dst, addr[:], fl, table)
}

func (p *InfinityProtocol) Read(dst uint16, addr InfinityTableAddr, params interface{}) bool {
	return p.send(dst, opREAD, addr[:], params)
}

func (p *InfinityProtocol) ReadTable(dst uint16, table InfinityTable) bool {
	addr := table.addr()
	return p.send(dst, opREAD, addr[:], table)
}

func (p *InfinityProtocol) sendFrame(buf []byte) bool {
	// Ensure we're not in the middle of reopening the serial port due to an error.
	if p.port == nil {
		return false
	}

	log.Debugf("transmitting frame: %x", buf)
	_, err := p.port.Write(buf)
	if err != nil {
		log.Errorf("error writing to serial: %s", err.Error())
		p.port.Close()
		p.port = nil
		return false
	}
	return true
}

func (p *InfinityProtocol) snoopResponse(srcMin uint16, srcMax uint16, cb snoopCallback) {
	s := InfinityProtocolSnoop{srcMin: srcMin, srcMax: srcMax, cb: cb}
	p.snoops = append(p.snoops, s)
}
