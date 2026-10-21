package thumbnail

import (
	"container/list"
	"context"
	"errors"
	"os"
	"sync"
)

type cacheKey struct {
	path           string
	size, modified int64
}
type cached struct {
	key  cacheKey
	info os.FileInfo
	data string
}

// Cache bounds retained previews and allows only two concurrent image decodes.
// Callers must authorize the path before every Get, even for cache hits.
type Cache struct {
	mu      sync.Mutex
	entries map[cacheKey]*list.Element
	recent  list.List
	bytes   int
	slots   chan struct{}
}

func NewCache() *Cache {
	return &Cache{entries: make(map[cacheKey]*list.Element), slots: make(chan struct{}, 2)}
}
func (c *Cache) lookup(key cacheKey, info os.FileInfo) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	item := entry.Value.(cached)
	if !os.SameFile(item.info, info) {
		return "", false
	}
	c.recent.MoveToFront(entry)
	return item.data, true
}

// Get regenerates changed files and keeps at most 192 images / 8 MiB in memory.
func (c *Cache) Get(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	key := cacheKey{path, info.Size(), info.ModTime().UnixNano()}
	if data, ok := c.lookup(key, info); ok {
		return data, nil
	}
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-c.slots }()
	if data, ok := c.lookup(key, info); ok {
		return data, nil
	}
	data, err := Render(ctx, path)
	if err != nil {
		return "", err
	}
	latest, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, latest) || info.Size() != latest.Size() || !info.ModTime().Equal(latest.ModTime()) {
		return "", errors.New("thumbnail source changed")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous, ok := c.entries[key]; ok {
		c.bytes -= len(previous.Value.(cached).data)
		c.recent.Remove(previous)
	}
	c.entries[key] = c.recent.PushFront(cached{key, latest, data})
	c.bytes += len(data)
	for len(c.entries) > 192 || c.bytes > 8*1024*1024 {
		last := c.recent.Back()
		item := last.Value.(cached)
		delete(c.entries, item.key)
		c.bytes -= len(item.data)
		c.recent.Remove(last)
	}
	return data, nil
}
