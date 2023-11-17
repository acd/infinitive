package dispatcher

type Listener struct {
	ch        chan *Event
	closeFunc func()
}

func (l *Listener) Receive() <-chan *Event {
	return l.ch
}

func (l *Listener) Close() {
	l.closeFunc()
}

type Dispatcher struct {
	listeners    map[*Listener]bool
	broadcast    chan *Event
	registerCh   chan *Listener
	deregisterCh chan *Listener
}

func New() *Dispatcher {
	d := &Dispatcher{
		broadcast:    make(chan *Event, 64),
		registerCh:   make(chan *Listener),
		deregisterCh: make(chan *Listener),
		listeners:    make(map[*Listener]bool),
	}
	go d.run()
	return d
}

type Event struct {
	Source string
	Data   interface{}
}

func (d *Dispatcher) NewListener() *Listener {
	l := &Listener{
		ch: make(chan *Event, 32),
	}
	l.closeFunc = func() {
		d.deregisterCh <- l
	}

	d.registerCh <- l
	return l
}

func (d *Dispatcher) BroadcastEvent(source string, data interface{}) {
	d.broadcast <- &Event{source, data}
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
