package infinity

import (
	"bytes"
	"encoding/binary"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tarm/serial"
)

const (
	DevTSTAT = uint16(0x2001)
	DevSAM   = uint16(0x9201)
)

const responseTimeout = 200 * time.Millisecond
const responseRetries = 5

type rawRequest struct {
	Data *[]byte
}

type protocolSnoop struct {
	srcMin uint16
	srcMax uint16
	cb     func(Frame)
}

type snoopList []protocolSnoop

type Protocol struct {
	device      string
	readTimeout time.Duration
	port        *serial.Port
	responseCh  chan Frame
	actionCh    chan *Action
	snoops      snoopList
	mu          sync.Mutex
}

func NewProtocol(device string) (*Protocol, error) {
	p := &Protocol{
		device:      device,
		readTimeout: time.Second * 5,
		responseCh:  make(chan Frame, 32),
		actionCh:    make(chan *Action),
	}
	if err := p.openSerial(); err != nil {
		return nil, err
	}

	go p.reader()
	go p.broker()

	return p, nil
}

type Action struct {
	requestFrame  Frame
	responseFrame *Frame
	ok            bool
	ch            chan bool
}

func (l snoopList) handle(frame Frame) {
	for _, s := range l {
		if frame.src >= s.srcMin && frame.src <= s.srcMax {
			s.cb(frame.Clone())
		}
	}
}

func (p *Protocol) openSerial() error {
	log.Printf("opening serial interface: %s", p.device)
	if p.port != nil {
		p.port.Close()
		p.port = nil
	}

	c := &serial.Config{
		Name:        p.device,
		Baud:        38400,
		ReadTimeout: p.readTimeout,
	}
	var err error
	p.port, err = serial.OpenPort(c)
	if err != nil {
		return err
	}

	return nil
}

func (p *Protocol) handleFrame(frame Frame) *Frame {
	log.Printf("read frame: %s", frame)

	switch frame.op {
	case Ack06:
		if frame.dst == DevSAM {
			p.responseCh <- frame
		}

		if len(frame.data) > 3 {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.snoops.handle(frame)
		}
	case WriteTableBlock:
		if frame.src == DevTSTAT && frame.dst == DevSAM {
			return &writeAck
		}
	}

	return nil
}

func (p *Protocol) reader() {
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

			frame := Frame{}
			if frame.decode(buf) {
				if response := p.handleFrame(frame); response != nil {
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

func (p *Protocol) broker() {
	defer panic("exiting InfinityProtocol broker, this should never happen")

	for action := range p.actionCh {
		p.performAction(action)
	}
}

func (p *Protocol) performAction(action *Action) {
	log.Infof("encoded frame: %s", action.requestFrame)
	encodedFrame := action.requestFrame.encode()
	p.sendFrame(encodedFrame)

	ticker := time.NewTicker(responseTimeout)
	defer ticker.Stop()

	for tries := 0; tries < responseRetries; {
		select {
		case res := <-p.responseCh:
			if res.src != action.requestFrame.dst {
				continue
			}

			reqTable := action.requestFrame.data[0:3]
			resTable := res.data[0:3]

			if action.requestFrame.op == ReadTableBlock && !bytes.Equal(reqTable, resTable) {
				log.Printf("got response for incorrect table, is: %x expected: %x", resTable, reqTable)
				continue
			}

			action.responseFrame = &res
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

func (p *Protocol) send(dst uint16, op uint8, requestData []byte, response interface{}) bool {
	f := Frame{src: DevSAM, dst: dst, op: op, data: requestData}
	act := &Action{requestFrame: f, ch: make(chan bool)}

	// Send action to action handling goroutine
	p.actionCh <- act
	// Wait for response
	ok := <-act.ch

	if ok && op == ReadTableBlock && act.responseFrame != nil && act.responseFrame.data != nil && len(act.responseFrame.data) > 6 {
		raw, ok := response.(rawRequest)
		if ok {
			log.Printf(">>>> handling a RawRequest")
			*raw.Data = append(*raw.Data, act.responseFrame.data[6:]...)
			log.Printf("raw data length is: %d", len(*raw.Data))
		} else {
			r := bytes.NewReader(act.responseFrame.data[6:])
			binary.Read(r, binary.BigEndian, response)
		}
		// log.Printf("%+v", data)
	}

	return ok
}

func (p *Protocol) Write(dst uint16, table []byte, addr []byte, params interface{}) bool {
	buf := new(bytes.Buffer)
	buf.Write(table[:])
	buf.Write(addr[:])
	binary.Write(buf, binary.BigEndian, params)

	return p.send(dst, WriteTableBlock, buf.Bytes(), nil)
}

func (p *Protocol) WriteTable(dst uint16, table Table, flags uint8) bool {
	addr := table.addr()
	fl := []byte{0x00, 0x00, flags}
	return p.Write(dst, addr[:], fl, table)
}

func (p *Protocol) Read(dst uint16, addr TableAddr, params interface{}) bool {
	return p.send(dst, ReadTableBlock, addr[:], params)
}

func (p *Protocol) ReadTable(dst uint16, table Table) bool {
	addr := table.addr()
	return p.send(dst, ReadTableBlock, addr[:], table)
}

func (p *Protocol) sendFrame(buf []byte) bool {
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

func (p *Protocol) SnoopResponse(srcMin uint16, srcMax uint16, cb func(Frame)) {
	s := protocolSnoop{
		srcMin: srcMin,
		srcMax: srcMax,
		cb:     cb,
	}

	p.mu.Lock()
	p.snoops = append(p.snoops, s)
	p.mu.Unlock()
}
