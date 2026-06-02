package clicore

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// ProgressBar tracks progress for human-readable output
type ProgressBar struct {
	mu              sync.RWMutex
	operation       string // Overall operation name
	totalItems      int
	completedItems  int
	currentItem     string
	currentItemSize int
	currentItemDone int
	done            chan struct{}
	stderr          io.Writer
}

func NewProgressBar(operation string, totalItems int, stderr io.Writer) *ProgressBar {
	return &ProgressBar{
		operation:  operation,
		totalItems: totalItems,
		stderr:     stderr,
		done:       make(chan struct{}),
	}
}

func (p *ProgressBar) StartDisplay() {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-p.done:
				// Final message - clear both lines and print completion
				p.mu.RLock()
				msg := fmt.Sprintf("\r\033[K✓ %s: %d/%d items completed\n\033[K",
					p.operation, p.completedItems, p.totalItems)
				p.mu.RUnlock()
				fmt.Fprint(p.stderr, msg)
				return
			case <-ticker.C:
				p.mu.RLock()
				// Overall progress bar
				var overallPercent float64
				if p.totalItems > 0 {
					overallPercent = float64(p.completedItems) / float64(p.totalItems) * 100
				}
				barWidth := 20
				filled := int(overallPercent / 100 * float64(barWidth))
				overallBar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

				// Line 1: Overall progress
				line1 := fmt.Sprintf("%s [%s] %.0f%% (%d/%d)",
					p.operation, overallBar, overallPercent, p.completedItems, p.totalItems)

				// Line 2: Item progress (if an item is in progress)
				var line2 string
				if p.currentItem != "" && p.currentItemSize > 0 {
					itemPercent := float64(p.currentItemDone) / float64(p.currentItemSize) * 100
					itemFilled := int(itemPercent / 100 * float64(barWidth))
					itemBar := strings.Repeat("█", itemFilled) + strings.Repeat("░", barWidth-itemFilled)
					line2 = fmt.Sprintf("  %s [%s] %.0f%%",
						p.currentItem, itemBar, itemPercent)
				} else {
					line2 = "" // Empty line if no current item
				}

				p.mu.RUnlock()

				// Print two lines: move to start, clear, print line1, newline, clear, print line2
				// Then move cursor back up to first line for next update
				fmt.Fprintf(p.stderr, "\r\033[K%s\n\033[K%s\033[1A", line1, line2)
			}
		}
	}()
}

func (p *ProgressBar) UpdateItem(name string, size int, done int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentItem = name
	p.currentItemSize = size
	p.currentItemDone = done
	if done >= size {
		p.completedItems++
		p.currentItem = ""
		p.currentItemSize = 0
		p.currentItemDone = 0
	}
}

func (p *ProgressBar) Done() {
	close(p.done)
}
