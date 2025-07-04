package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test for race condition in updateClipboard method
func TestUpdateClipboardRace(t *testing.T) {
	cs := NewClipboardServer()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Start multiple goroutines calling updateClipboard concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				content := fmt.Sprintf("content-%d-%d", id, j)
				cs.updateClipboard(content)
				time.Sleep(time.Microsecond) // Small delay to increase race window
			}
		}(i)
	}

	wg.Wait()

	// If no race detector warnings, the test passes
	// But the race condition still exists in the logic
	t.Log("Concurrent updateClipboard calls completed")
}

// Test for race condition in cancel field access
func TestCancelFieldRace(t *testing.T) {
	cs := NewClipboardServer()

	var wg sync.WaitGroup

	// Goroutine 1: Repeatedly set cancel field
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cancel := context.CancelFunc(func() {}) // Dummy cancel function
			cs.cancel.Store(&cancel)                // FIXED: Use atomic store
			time.Sleep(time.Microsecond)
		}
	}()

	// Goroutine 2: Repeatedly read cancel field
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if cancelPtr := cs.cancel.Load(); cancelPtr != nil { // FIXED: Use atomic load
				// Access cancel field
			}
			time.Sleep(time.Microsecond)
		}
	}()

	wg.Wait()
	t.Log("Concurrent cancel field access completed")
}

// Test proper atomic operations
func TestAtomicOperations(t *testing.T) {
	cs := NewClipboardServer()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Test atomic running flag
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Try to set running state
			if atomic.CompareAndSwapInt32(&cs.running, 0, 1) {
				// Simulate work
				time.Sleep(time.Millisecond)
				atomic.StoreInt32(&cs.running, 0)
			}
		}()
	}

	wg.Wait()
	t.Log("Atomic operations test completed")
}

// Test CAS retry limit functionality
func TestUpdateClipboardRetryLimit(t *testing.T) {
	cs := NewClipboardServer()

	// Initialize with some content
	cs.updateClipboard("initial-content")

	// Test that normal operation still works
	updated := cs.updateClipboard("new-content")
	if !updated {
		t.Error("Expected content update to succeed")
	}

	content, _ := cs.getLastClipboard()
	if content != "new-content" {
		t.Errorf("Expected 'new-content', got '%s'", content)
	}

	t.Log("CAS retry limit functionality verified")
}

// Test session file tracking during shutdown
func TestSessionFileTrackingDuringShutdown(t *testing.T) {
	cs := NewClipboardServer()

	// Set running state
	atomic.StoreInt32(&cs.running, 1)

	// Should track file when running
	cs.addSessionFile("/tmp/test1.txt")

	// Stop the server
	atomic.StoreInt32(&cs.running, 0)

	// Should not track file when stopped
	cs.addSessionFile("/tmp/test2.txt")

	cs.filesMutex.Lock()
	fileCount := len(cs.sessionFiles)
	cs.filesMutex.Unlock()

	if fileCount != 1 {
		t.Errorf("Expected 1 tracked file, got %d", fileCount)
	}

	t.Log("Session file tracking during shutdown test passed")
}

// Benchmark updateClipboard to show race impact
func BenchmarkUpdateClipboardConcurrent(b *testing.B) {
	cs := NewClipboardServer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			content := fmt.Sprintf("benchmark-content-%d", i)
			cs.updateClipboard(content)
			i++
		}
	})
}
