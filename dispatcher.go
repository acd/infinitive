package main

import (
	"encoding/json"
)

type EventListener struct {
	ch chan []byte
}

type EventDispatcher struct {
	listeners  map[*EventListener]bool
	broadcast  chan []byte
	register   chan *EventListener
	deregister chan *EventListener
}

var Dispatcher *EventDispatcher = newEventDispatcher()

func newEventDispatcher() *EventDispatcher {
	return &EventDispatcher{
		broadcast:  make(chan []byte, 64),
		register:   make(chan *EventListener),
		deregister: make(chan *EventListener),
		listeners:  make(map[*EventListener]bool),
	}
}

func (d *EventDispatcher) dispatch(msg []byte) {
	d.broadcast <- msg
}

type broadcastEvent struct {
	Source string      `json:"source"`
	Data   interface{} `json:"data"`
}

func serializeEvent(source string, data interface{}) []byte {
	msg, _ := json.Marshal(&broadcastEvent{Source: source, Data: data})
	return msg
}

func (d *EventDispatcher) broadcastEvent(source string, data interface{}) {
	d.broadcast <- serializeEvent(source, data)
}

func (h *EventDispatcher) run() {
	for {
		select {
		case listener := <-h.register:
			h.listeners[listener] = true
		case listener := <-h.deregister:
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

func init() {
	go Dispatcher.run()
}
