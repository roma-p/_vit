package clicore

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a thread-safe buffer for testing
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestProgressBar_UpdateItem_InProgress(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar("Copying", 3, &buf)

	// Simulate partial progress on an item
	pb.UpdateItem("file1.txt", 100, 50)

	pb.mu.RLock()
	defer pb.mu.RUnlock()

	if pb.currentItem != "file1.txt" {
		t.Errorf("expected currentItem 'file1.txt', got '%s'", pb.currentItem)
	}
	if pb.currentItemSize != 100 {
		t.Errorf("expected currentItemSize 100, got %d", pb.currentItemSize)
	}
	if pb.currentItemDone != 50 {
		t.Errorf("expected currentItemDone 50, got %d", pb.currentItemDone)
	}
	if pb.completedItems != 0 {
		t.Errorf("expected completedItems 0, got %d", pb.completedItems)
	}
}

func TestProgressBar_UpdateItem_Completed(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar("Copying", 3, &buf)

	// Complete an item (done >= size)
	pb.UpdateItem("file1.txt", 100, 100)

	pb.mu.RLock()
	defer pb.mu.RUnlock()

	// After completion, current item should be cleared
	if pb.currentItem != "" {
		t.Errorf("expected currentItem empty after completion, got '%s'", pb.currentItem)
	}
	if pb.currentItemSize != 0 {
		t.Errorf("expected currentItemSize 0 after completion, got %d", pb.currentItemSize)
	}
	if pb.completedItems != 1 {
		t.Errorf("expected completedItems 1, got %d", pb.completedItems)
	}
}

func TestProgressBar_UpdateItem_MultipleItems(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar("Copying", 3, &buf)

	// Complete 3 items
	pb.UpdateItem("file1.txt", 100, 100)
	pb.UpdateItem("file2.txt", 200, 200)
	pb.UpdateItem("file3.txt", 50, 50)

	pb.mu.RLock()
	defer pb.mu.RUnlock()

	if pb.completedItems != 3 {
		t.Errorf("expected completedItems 3, got %d", pb.completedItems)
	}
}

func TestProgressBar_Done_WritesCompletionMessage(t *testing.T) {
	var buf syncBuffer
	pb := NewProgressBar("Copying", 2, &buf)

	// Complete some items
	pb.UpdateItem("file1.txt", 100, 100)
	pb.UpdateItem("file2.txt", 100, 100)

	// Start display and immediately signal done
	pb.StartDisplay()
	pb.Done()

	// Wait for goroutine to write final message
	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	if !strings.Contains(output, "✓") {
		t.Errorf("expected checkmark in output, got: %s", output)
	}
	if !strings.Contains(output, "Copying") {
		t.Errorf("expected operation name in output, got: %s", output)
	}
	if !strings.Contains(output, "2/2") {
		t.Errorf("expected '2/2' completion count in output, got: %s", output)
	}
}

func TestProgressBar_Display_ShowsProgress(t *testing.T) {
	var buf syncBuffer
	pb := NewProgressBar("Copying", 2, &buf)

	pb.StartDisplay()

	// Simulate item in progress
	pb.UpdateItem("bigfile.bin", 1000, 500)

	// Wait for at least one tick (100ms)
	time.Sleep(150 * time.Millisecond)

	pb.Done()
	time.Sleep(50 * time.Millisecond)

	output := buf.String()

	// Should show operation name
	if !strings.Contains(output, "Copying") {
		t.Errorf("expected 'Copying' in output, got: %s", output)
	}

	// Should show item name at some point
	if !strings.Contains(output, "bigfile.bin") {
		t.Errorf("expected 'bigfile.bin' in output, got: %s", output)
	}

	// Should contain progress bar characters
	if !strings.Contains(output, "█") && !strings.Contains(output, "░") {
		t.Errorf("expected progress bar characters in output, got: %s", output)
	}
}

func TestProgressBar_ZeroTotalItems(t *testing.T) {
	var buf syncBuffer
	pb := NewProgressBar("Copying", 0, &buf)

	pb.StartDisplay()

	// With 0 total items, completing items should still work
	pb.UpdateItem("file1.txt", 100, 100)

	time.Sleep(150 * time.Millisecond)

	pb.Done()
	time.Sleep(50 * time.Millisecond)

	// Should not panic and should complete
	output := buf.String()
	if !strings.Contains(output, "✓") {
		t.Errorf("expected checkmark in final output, got: %s", output)
	}
}

func TestProgressBar_ConcurrentUpdates(t *testing.T) {
	var buf syncBuffer
	pb := NewProgressBar("Copying", 100, &buf)

	pb.StartDisplay()

	// Simulate concurrent updates (shouldn't cause race conditions)
	done := make(chan struct{})
	for i := range 10 {
		go func(n int) {
			for range 10 {
				pb.UpdateItem("file.txt", 100, 100)
			}
			done <- struct{}{}
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	pb.Done()
	time.Sleep(50 * time.Millisecond)

	pb.mu.RLock()
	defer pb.mu.RUnlock()

	if pb.completedItems != 100 {
		t.Errorf("expected 100 completed items, got %d", pb.completedItems)
	}
}
