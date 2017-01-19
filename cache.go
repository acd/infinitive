package main

import (
	"reflect"
	"sync"
)

type Cache map[string]interface{}

var cache Cache = make(Cache)
var mutex = &sync.Mutex{}

func (c Cache) update(name string, data interface{}) {
	mutex.Lock()
	defer mutex.Unlock()

	old := c[name]
	if !reflect.DeepEqual(old, data) {
		Dispatcher.broadcastEvent(name, data)
		c[name] = data
	}
}

func (c Cache) get(name string) interface{} {
	mutex.Lock()
	defer mutex.Unlock()

	return c[name]
}

func (c Cache) dump() Cache {
	mutex.Lock()
	defer mutex.Unlock()

	n := make(Cache)
	for k, v := range c {
		c[k] = v
	}
	return n
}
