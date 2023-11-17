package dispatcher

import "context"

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
	ctx          context.Context
	listeners    map[*Listener]struct{}
	broadcast    chan *Event
	registerCh   chan *Listener
	deregisterCh chan *Listener
}

func New(ctx context.Context) *Dispatcher {
	d := &Dispatcher{
		ctx:          ctx,
		broadcast:    make(chan *Event, 64),
		registerCh:   make(chan *Listener),
		deregisterCh: make(chan *Listener),
		listeners:    make(map[*Listener]struct{}),
	}
	go d.run()
	return d
}

type Event struct {
	Source string
	Data   any
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

func (d *Dispatcher) BroadcastEvent(source string, data any) {
	d.broadcast <- &Event{source, data}
}

func (d *Dispatcher) run() {
	for {
		select {
		case listener := <-d.registerCh:
			d.listeners[listener] = struct{}{}
		case listener := <-d.deregisterCh:
			if _, ok := d.listeners[listener]; ok {
				delete(d.listeners, listener)
				close(listener.ch)
			}
		case message := <-d.broadcast:
			for listener := range d.listeners {
				select {
				case listener.ch <- message:
				default:
					close(listener.ch)
					delete(d.listeners, listener)
				}
			}
		case <-d.ctx.Done():
			return
		}
	}
}
