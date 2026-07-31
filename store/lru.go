package store

import "container/list"

type warmValue struct {
	key  string
	data []byte
}

type warmCache struct {
	entries map[string]*list.Element
	order   list.List
	bytes   int
	limit   int
	count   int
}

func newWarmCache(count, limit int) warmCache {
	return warmCache{entries: make(map[string]*list.Element), count: count, limit: limit}
}

func (c *warmCache) get(key string) ([]byte, bool) {
	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(element)
	return element.Value.(warmValue).data, true
}

func (c *warmCache) put(key string, data []byte) {
	if c.count == 0 || len(data) > c.limit {
		return
	}
	owned := append([]byte(nil), data...)
	if element, ok := c.entries[key]; ok {
		value := element.Value.(warmValue)
		c.bytes -= len(value.data)
		value.data = owned
		element.Value = value
		c.bytes += len(owned)
		c.order.MoveToFront(element)
	} else {
		element := c.order.PushFront(warmValue{key: key, data: owned})
		c.entries[key] = element
		c.bytes += len(owned)
	}
	for len(c.entries) > c.count || c.bytes > c.limit {
		element := c.order.Back()
		value := element.Value.(warmValue)
		delete(c.entries, value.key)
		c.bytes -= len(value.data)
		c.order.Remove(element)
	}
}

func (c *warmCache) remove(key string) {
	element, ok := c.entries[key]
	if !ok {
		return
	}
	value := element.Value.(warmValue)
	delete(c.entries, key)
	c.bytes -= len(value.data)
	c.order.Remove(element)
}
