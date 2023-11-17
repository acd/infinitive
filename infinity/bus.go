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

// For future use in unit tests
type port interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error
}

type Bus struct {
	device      string
	readTimeout time.Duration
	port        port
	responseCh  chan Frame
	actionCh    chan *Action
	snoops      []frameHandler
	mu          sync.Mutex
}

func NewBus(device string) (*Bus, error) {
	b := &Bus{
		device:      device,
		readTimeout: time.Second * 5,
		responseCh:  make(chan Frame, 32),
		actionCh:    make(chan *Action),
	}
	if err := b.openSerial(); err != nil {
		return nil, err
	}

	go b.reader()
	go b.broker()

	return b, nil
}

type Action struct {
	requestFrame  Frame
	responseFrame *Frame
	ok            bool
	ch            chan bool
}

func (b *Bus) openSerial() error {
	log.Printf("opening serial interface: %s", b.device)
	if b.port != nil {
		b.port.Close()
		b.port = nil
	}

	c := &serial.Config{
		Name:        b.device,
		Baud:        38400,
		ReadTimeout: b.readTimeout,
	}
	var err error
	b.port, err = serial.OpenPort(c)
	return err
}

func (b *Bus) handleFrame(frame Frame) *Frame {
	log.Printf("read frame: %s", frame)

	switch frame.op {
	case Ack06:
		if frame.dst == DevSAM {
			b.responseCh <- frame
		}

		if len(frame.data) > 3 {
			b.mu.Lock()
			defer b.mu.Unlock()
			for _, snoop := range b.snoops {
				snoop(frame)
			}
		}
	case WriteTableBlock:
		if frame.src == DevTSTAT && frame.dst == DevSAM {
			return &writeAck
		}
	}

	return nil
}

func (b *Bus) reader() {
	defer panic("exiting InfinityProtocol reader, this should never happen")

	msg := []byte{}
	buf := make([]byte, 1024)

	for {
		if b.port == nil {
			msg = []byte{}
			b.openSerial()
		}

		n, err := b.port.Read(buf)
		if n == 0 || err != nil {
			log.Printf("error reading from serial port: %s", err.Error())
			if b.port != nil {
				b.port.Close()
			}
			b.port = nil
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
				if response := b.handleFrame(frame); response != nil {
					b.sendFrame(response.encode())
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

func (b *Bus) broker() {
	defer panic("exiting InfinityProtocol broker, this should never happen")

	for action := range b.actionCh {
		b.performAction(action)
	}
}

func (b *Bus) performAction(action *Action) {
	log.Infof("encoded frame: %s", action.requestFrame)
	encodedFrame := action.requestFrame.encode()
	b.sendFrame(encodedFrame)

	ticker := time.NewTicker(responseTimeout)
	defer ticker.Stop()

	for tries := 0; tries < responseRetries; {
		select {
		case res := <-b.responseCh:
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
			b.sendFrame(encodedFrame)
			tries++
		}
	}

	log.Printf("action timed out")
	action.ch <- false
}

func (b *Bus) send(dst uint16, op uint8, requestData []byte, response interface{}) bool {
	f := Frame{src: DevSAM, dst: dst, op: op, data: requestData}
	act := &Action{requestFrame: f, ch: make(chan bool)}

	// Send action to action handling goroutine
	b.actionCh <- act
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

func (b *Bus) Write(dst uint16, table []byte, addr []byte, params interface{}) bool {
	buf := new(bytes.Buffer)
	buf.Write(table[:])
	buf.Write(addr[:])
	binary.Write(buf, binary.BigEndian, params)

	return b.send(dst, WriteTableBlock, buf.Bytes(), nil)
}

func (b *Bus) WriteTable(dst uint16, table Table, flags uint8) bool {
	addr := table.addr()
	fl := []byte{0x00, 0x00, flags}
	return b.Write(dst, addr[:], fl, table)
}

func (b *Bus) Read(dst uint16, addr TableAddr, params interface{}) bool {
	return b.send(dst, ReadTableBlock, addr[:], params)
}

func (b *Bus) ReadTable(dst uint16, table Table) bool {
	addr := table.addr()
	return b.send(dst, ReadTableBlock, addr[:], table)
}

func (b *Bus) sendFrame(buf []byte) bool {
	// Ensure we're not in the middle of reopening the serial port due to an error.
	if b.port == nil {
		return false
	}

	log.Debugf("transmitting frame: %x", buf)
	_, err := b.port.Write(buf)
	if err != nil {
		log.Errorf("error writing to serial: %s", err.Error())
		b.port.Close()
		b.port = nil
		return false
	}
	return true
}

func (b *Bus) SnoopResponse(f func(Frame)) {
	b.mu.Lock()
	b.snoops = append(b.snoops, f)
	b.mu.Unlock()
}

func filter(p framePredicate, fn frameHandler) frameHandler {
	return func(f Frame) {
		if p(f) {
			fn(f)
		}
	}
}
