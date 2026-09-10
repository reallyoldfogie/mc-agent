package utils

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingBufferNewCreatesEmptyBuffer(t *testing.T) {
	rb := NewRingBuffer[int](5)
	assert.Equal(t, 0, rb.Len())
	assert.Equal(t, 5, rb.Cap())
	assert.Empty(t, rb.Items())
}

func TestRingBufferAddBelowCapacity(t *testing.T) {
	rb := NewRingBuffer[int](5)

	rb.Add(1)
	assert.Equal(t, 1, rb.Len())
	assert.Equal(t, []int{1}, rb.Items())

	rb.Add(2)
	assert.Equal(t, 2, rb.Len())
	assert.Equal(t, []int{1, 2}, rb.Items())

	rb.Add(3)
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, []int{1, 2, 3}, rb.Items())
}

func TestRingBufferAddAtCapacity(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, []int{1, 2, 3}, rb.Items())
}

func TestRingBufferWrapsAroundWhenFull(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	// Buffer now full: [1, 2, 3], writeIndex=0

	rb.Add(4)
	// Overwrites 1: [4, 2, 3], writeIndex=1. Items in insertion order: 2, 3, 4 (oldest to newest)
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, []int{2, 3, 4}, rb.Items())

	rb.Add(5)
	// Overwrites 2: [4, 5, 3], writeIndex=2. Items in insertion order: 3, 4, 5
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, []int{3, 4, 5}, rb.Items())

	rb.Add(6)
	// Overwrites 3: [4, 5, 6], writeIndex=0. Items in insertion order: 4, 5, 6
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, []int{4, 5, 6}, rb.Items())
}

func TestRingBufferMaintainsInsertionOrder(t *testing.T) {
	rb := NewRingBuffer[int](4)

	for i := 1; i <= 10; i++ {
		rb.Add(i)
	}

	// After adding 1-10 to a buffer of size 4, should contain 7,8,9,10
	assert.Equal(t, 4, rb.Len())
	assert.Equal(t, []int{7, 8, 9, 10}, rb.Items())
}

func TestRingBufferWithStrings(t *testing.T) {
	rb := NewRingBuffer[string](3)

	rb.Add("a")
	rb.Add("b")
	rb.Add("c")
	assert.Equal(t, []string{"a", "b", "c"}, rb.Items())

	rb.Add("d")
	rb.Add("e")
	assert.Equal(t, []string{"c", "d", "e"}, rb.Items())
}

func TestRingBufferItemsReturnsACopy(t *testing.T) {
	rb := NewRingBuffer[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	items1 := rb.Items()
	items2 := rb.Items()

	// Should be equal but different slices
	assert.Equal(t, items1, items2)
	assert.NotSame(t, &items1[0], &items2[0])

	// Modifying one shouldn't affect the other
	items1[0] = 999
	items2 = rb.Items()
	assert.Equal(t, 1, items2[0])
}

func TestRingBufferCapacityOne(t *testing.T) {
	rb := NewRingBuffer[int](1)

	rb.Add(1)
	assert.Equal(t, []int{1}, rb.Items())

	rb.Add(2)
	assert.Equal(t, 1, rb.Len())
	assert.Equal(t, []int{2}, rb.Items())

	rb.Add(3)
	assert.Equal(t, 1, rb.Len())
	assert.Equal(t, []int{3}, rb.Items())
}

func TestRingBufferLargeCapacity(t *testing.T) {
	rb := NewRingBuffer[int](1000)

	// Fill to capacity
	for i := 0; i < 1000; i++ {
		rb.Add(i)
	}
	assert.Equal(t, 1000, rb.Len())

	// Add one more, should wrap
	rb.Add(1000)
	assert.Equal(t, 1000, rb.Len())

	items := rb.Items()
	assert.Len(t, items, 1000)
	assert.Equal(t, 1, items[0])      // Oldest
	assert.Equal(t, 1000, items[999]) // Newest
}

func TestRingBufferWithStructs(t *testing.T) {
	type Point struct {
		X, Y int
	}

	rb := NewRingBuffer[Point](2)

	rb.Add(Point{1, 2})
	rb.Add(Point{3, 4})
	rb.Add(Point{5, 6})

	items := rb.Items()
	assert.Equal(t, 2, len(items))
	assert.Equal(t, Point{3, 4}, items[0])
	assert.Equal(t, Point{5, 6}, items[1])
}

func TestRingBufferConcurrentAdds(t *testing.T) {
	rb := NewRingBuffer[int](100)
	const numGoroutines = 10
	const itemsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < itemsPerGoroutine; i++ {
				rb.Add(id*1000 + i)
			}
		}(g)
	}

	wg.Wait()

	// After 1000 adds to a buffer of 100, should contain the last 100
	assert.Equal(t, 100, rb.Len())
	assert.Equal(t, 100, rb.Cap())

	items := rb.Items()
	assert.Len(t, items, 100)
	// All items should be valid (no corruption from concurrent access)
	for _, item := range items {
		assert.Greater(t, item, -1)
		assert.Less(t, item, 10000)
	}
}

func TestRingBufferConcurrentReadsAndWrites(t *testing.T) {
	rb := NewRingBuffer[int](50)
	const numWriters = 5
	const numReaders = 5
	const operationsPerGoroutine = 100
	done := make(chan struct{})

	// Writers
	for w := 0; w < numWriters; w++ {
		go func(id int) {
			for i := 0; i < operationsPerGoroutine; i++ {
				rb.Add(id*10000 + i)
			}
			done <- struct{}{}
		}(w)
	}

	// Readers
	for r := 0; r < numReaders; r++ {
		go func() {
			for i := 0; i < operationsPerGoroutine; i++ {
				items := rb.Items()
				// Just verify we get a valid slice, no panics or corrupted data
				assert.True(t, len(items) <= 50)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numWriters+numReaders; i++ {
		<-done
	}

	// Final state should be valid
	items := rb.Items()
	assert.True(t, len(items) <= 50)
	assert.Equal(t, 50, rb.Cap())
}

func TestRingBufferConcurrentLenAndCap(t *testing.T) {
	rb := NewRingBuffer[int](25)
	const numOps = 1000

	var wg sync.WaitGroup
	wg.Add(3)

	// Writer
	go func() {
		defer wg.Done()
		for i := 0; i < numOps; i++ {
			rb.Add(i)
		}
	}()

	// Len reader
	go func() {
		defer wg.Done()
		for i := 0; i < numOps; i++ {
			len := rb.Len()
			assert.True(t, len >= 0 && len <= 25)
		}
	}()

	// Cap reader
	go func() {
		defer wg.Done()
		for i := 0; i < numOps; i++ {
			cap := rb.Cap()
			assert.Equal(t, 25, cap)
		}
	}()

	wg.Wait()
}
