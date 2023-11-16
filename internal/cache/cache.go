package cache

import (
	"maps"
	"reflect"
	"sync"
)

type Cache struct {
	data         map[string]any
	onUpdateFunc func(string, any)
	mu           sync.RWMutex
}

func New(onUpdateFunc func(string, any)) *Cache {
	return &Cache{
		data:         make(map[string]any),
		onUpdateFunc: onUpdateFunc,
	}
}

func (c *Cache) Update(name string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !reflect.DeepEqual(c.data[name], data) {
		c.data[name] = data
		if c.onUpdateFunc != nil {
			c.onUpdateFunc(name, data)
		}
	}
}

func (c *Cache) Get(name string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.data[name]
}

func (c *Cache) Dump() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return maps.Clone(c.data)
}
