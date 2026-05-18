package utils

import (
	"fmt"
	"strings"
	"sync"
)

// RingBuffer is a thread-safe, fixed-capacity, FIFO ring buffer that drops the oldest entry when full.
type RingBuffer[T any] struct {
	items      []T
	writeIndex int
	count      int
	mu         sync.RWMutex
}

// NewRingBuffer creates a new RingBuffer with the specified capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{items: make([]T, capacity)}
}

// Add appends item to the buffer. If the buffer is full the oldest entry is overwritten.
func (r *RingBuffer[T]) Add(item T) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[r.writeIndex] = item
	r.writeIndex = (r.writeIndex + 1) % len(r.items)
	if r.count < len(r.items) {
		r.count++
	}
}

// Items returns a copy of all buffered items in insertion order (oldest first).
func (r *RingBuffer[T]) Items() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]T, r.count)
	if r.count < len(r.items) {
		copy(result, r.items[:r.count])
	} else {
		for i := 0; i < r.count; i++ {
			result[i] = r.items[(r.writeIndex+i)%len(r.items)]
		}
	}
	return result
}

// Len returns the number of items currently in the buffer.
func (r *RingBuffer[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

// Cap returns the maximum number of items the buffer can hold.
func (r *RingBuffer[T]) Cap() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

func (r *RingBuffer[T]) String() string {
	var result strings.Builder

	for i, val := range r.items {
		if i >= r.count {
			break
		}
		if str, ok := any(val).(fmt.Stringer); ok {
			fmt.Fprintf(&result, "  %d: %s\n", i+1, str)
		} else {
			fmt.Fprintf(&result, "  %d: %v\n", i+1, val)
		}
	}
	return result.String()
}
