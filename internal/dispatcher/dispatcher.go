package dispatcher

import (
	"encoding/json"
)

type Listener struct {
	ch chan []byte
}

func (l *Listener) Receive() <-chan []byte {
	return l.ch
}

type Dispatcher struct {
	listeners    map[*Listener]bool
	broadcast    chan []byte
	registerCh   chan *Listener
	deregisterCh chan *Listener
}

func New() *Dispatcher {
	d := &Dispatcher{
		broadcast:    make(chan []byte, 64),
		registerCh:   make(chan *Listener),
		deregisterCh: make(chan *Listener),
		listeners:    make(map[*Listener]bool),
	}
	go d.run()
	return d
}

type broadcastEvent struct {
	Source string      `json:"source"`
	Data   interface{} `json:"data"`
}

func SerializeEvent(source string, data interface{}) []byte {
	msg, _ := json.Marshal(&broadcastEvent{Source: source, Data: data})
	return msg
}

func (d *Dispatcher) NewListener() *Listener {
	l := &Listener{make(chan []byte, 32)}
	d.registerCh <- l
	return l
}

func (d *Dispatcher) Deregister(l *Listener) {
	d.deregisterCh <- l
}

func (d *Dispatcher) BroadcastEvent(source string, data interface{}) {
	d.broadcast <- SerializeEvent(source, data)
}

func (h *Dispatcher) run() {
	for {
		select {
		case listener := <-h.registerCh:
			h.listeners[listener] = true
		case listener := <-h.deregisterCh:
			if _, ok := h.listeners[listener]; ok {
				delete(h.listeners, listener)
				close(listener.ch)
			}
		case message := <-h.broadcast:
			for listener := range h.listeners {
				select {
				case listener.ch <- message:
				default:
					close(listener.ch)
					delete(h.listeners, listener)
				}
			}
		}
	}
}
