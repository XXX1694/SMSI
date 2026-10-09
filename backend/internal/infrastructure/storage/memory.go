package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"
)

// Memory is an in-memory storage for tests and local runs without S3.
type Memory struct {
	mu      sync.RWMutex
	objects map[string][]byte
	types   map[string]string
}

// NewMemory creates an empty store.
func NewMemory() *Memory {
	return &Memory{objects: map[string][]byte{}, types: map[string]string{}}
}

// Put stores the object.
func (m *Memory) Put(_ context.Context, key string, r io.Reader, size int64, contentType string) error {
	b, err := io.ReadAll(io.LimitReader(r, size+1))
	if err != nil {
		return err
	}
	if int64(len(b)) != size {
		return fmt.Errorf("storage: size mismatch: got %d want %d", len(b), size)
	}
	m.mu.Lock()
	m.objects[key], m.types[key] = b, contentType
	m.mu.Unlock()
	return nil
}

// Get returns the object.
func (m *Memory) Get(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	b, ok := m.objects[key]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("storage: %s not found", key)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// Delete removes the object.
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.objects, key)
	delete(m.types, key)
	m.mu.Unlock()
	return nil
}

// PresignGet returns a fake URL (memory objects are not web-served).
func (m *Memory) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return "memory://" + url.PathEscape(key) + "?expires=" + time.Now().Add(ttl).UTC().Format(time.RFC3339), nil
}

// Ping always succeeds.
func (m *Memory) Ping(context.Context) error { return nil }

// Has reports whether key exists (tests).
func (m *Memory) Has(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.objects[key]
	return ok
}

// Len is the number of stored objects (tests).
func (m *Memory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.objects)
}
