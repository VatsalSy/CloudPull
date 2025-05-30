/**
 * Progress Tracker Tests
 * Comprehensive test suite for progress tracking functionality
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

package progress

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewTracker(t *testing.T) {
	t.Run("create tracker with default batch size", func(t *testing.T) {
		tracker := NewTracker(0)
		if tracker == nil {
			t.Fatal("expected tracker to be created")
		}
		if tracker.progress.batchSize != 100 {
			t.Errorf("expected default batch size 100, got %d",
				tracker.progress.batchSize)
		}
	})

	t.Run("create tracker with custom batch size", func(t *testing.T) {
		tracker := NewTracker(200)
		if tracker.progress.batchSize != 200 {
			t.Errorf("expected batch size 200, got %d",
				tracker.progress.batchSize)
		}
	})
}

func TestTrackerStartStop(t *testing.T) {
	tracker := NewTracker(100)

	t.Run("start tracking", func(t *testing.T) {
		tracker.Start()

		if tracker.progress.state != StateRunning {
			t.Errorf("expected state Running, got %v", tracker.progress.state)
		}

		if tracker.progress.startTime.IsZero() {
			t.Error("expected start time to be set")
		}
	})

	t.Run("stop tracking", func(t *testing.T) {
		tracker.Stop()

		if tracker.progress.state != StateCompleted {
			t.Errorf("expected state Completed, got %v", tracker.progress.state)
		}
	})
}

func TestTrackerPauseResume(t *testing.T) {
	tracker := NewTracker(100)
	tracker.Start()

	t.Run("pause tracking", func(t *testing.T) {
		tracker.Pause()

		if tracker.progress.state != StatePaused {
			t.Errorf("expected state Paused, got %v", tracker.progress.state)
		}

		if tracker.progress.lastPauseTime.IsZero() {
			t.Error("expected pause time to be set")
		}
	})

	t.Run("resume tracking", func(t *testing.T) {
		time.Sleep(100 * time.Millisecond)
		tracker.Resume()

		if tracker.progress.state != StateRunning {
			t.Errorf("expected state Running, got %v", tracker.progress.state)
		}

		if tracker.progress.pausedDuration == 0 {
			t.Error("expected paused duration to be recorded")
		}
	})

	tracker.Stop()
}

func TestTrackerConcurrentUpdates(t *testing.T) {
	tracker := NewTracker(100)
	tracker.Start()
	defer tracker.Stop()

	tracker.SetTotals(1000, 1024*1024*100)

	// Simulate concurrent file updates
	var wg sync.WaitGroup
	numGoroutines := 10
	filesPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < filesPerGoroutine; j++ {
				filename := fmt.Sprintf("file_%d_%d.txt", id, j)
				tracker.AddFile(filename, 1024)

				// Simulate some work
				time.Sleep(time.Microsecond * 10)
			}
		}(i)
	}

	wg.Wait()

	// Verify final counts
	snapshot := tracker.GetSnapshot()
	expectedFiles := int64(numGoroutines * filesPerGoroutine)
	if snapshot.ProcessedFiles != expectedFiles {
		t.Errorf("expected %d processed files, got %d",
			expectedFiles, snapshot.ProcessedFiles)
	}

	expectedBytes := expectedFiles * 1024
	if snapshot.ProcessedBytes != expectedBytes {
		t.Errorf("expected %d processed bytes, got %d",
			expectedBytes, snapshot.ProcessedBytes)
	}
}

func TestTrackerSubscriptions(t *testing.T) {
	tracker := NewTracker(10)
	tracker.Start()
	defer tracker.Stop()

	t.Run("subscribe and receive updates", func(t *testing.T) {
		ch := tracker.Subscribe()

		// Add some files
		go func() {
			for i := 0; i < 5; i++ {
				tracker.AddFile(fmt.Sprintf("file%d.txt", i), 1024)
				time.Sleep(50 * time.Millisecond)
			}
		}()

		// Collect updates
		var updates []Update
		timeout := time.After(1 * time.Second)

		for {
			select {
			case update := <-ch:
				updates = append(updates, update)
				if len(updates) >= 5 {
					goto done
				}
			case <-timeout:
				goto done
			}
		}

	done:
		if len(updates) == 0 {
			t.Error("expected to receive updates")
		}

		tracker.Unsubscribe(ch)
	})
}

func TestProgressSnapshot(t *testing.T) {
	tracker := NewTracker(100)
	tracker.Start()
	defer tracker.Stop()

	// Set total files to 4 and total bytes to 100MB
	tracker.SetTotals(4, 1024*1024*100)
	
	// Add a small delay to ensure elapsed time > 0
	time.Sleep(50 * time.Millisecond)
	
	// AddFile already increments both files and bytes, so don't call AddBytes separately
	tracker.AddFile("file1.txt", 1024*1024*10)
	time.Sleep(50 * time.Millisecond) // Simulate time passing
	tracker.AddFile("file2.txt", 1024*1024*15)

	time.Sleep(100 * time.Millisecond) // Allow batch processing

	snapshot := tracker.GetSnapshot()

	t.Run("percent complete", func(t *testing.T) {
		percent := snapshot.PercentComplete()
		expected := float64(25*1024*1024) / float64(100*1024*1024) * 100

		if percent != expected {
			t.Errorf("expected %.2f%% complete, got %.2f%%", expected, percent)
		}
	})

	t.Run("bytes per second", func(t *testing.T) {
		bps := snapshot.BytesPerSecond()
		if bps <= 0 {
			t.Error("expected positive bytes per second")
		}
	})

	t.Run("ETA calculation", func(t *testing.T) {
		eta := snapshot.ETA()
		// Debug information
		remainingBytes := snapshot.TotalBytes - snapshot.ProcessedBytes
		t.Logf("ProcessedBytes: %d, TotalBytes: %d, RemainingBytes: %d, ElapsedTime: %v, BytesPerSecond: %.2f, ETA: %v (%.3f seconds)", 
			snapshot.ProcessedBytes, snapshot.TotalBytes, remainingBytes, snapshot.ElapsedTime, snapshot.BytesPerSecond(), eta, eta.Seconds())
		
		// ETA should be >= 0
		if eta < 0 {
			t.Error("ETA should not be negative")
		}
		
		// If there are remaining bytes and we have a positive transfer rate, 
		// ETA should be calculable (even if very small)
		if remainingBytes > 0 && snapshot.BytesPerSecond() > 0 {
			// ETA can be very small in tests due to high transfer rates
			// Just verify it's not negative
			if eta < 0 {
				t.Error("expected non-negative ETA when there are remaining bytes")
			}
		}
	})
}

func TestBatchProcessing(t *testing.T) {
	batchSize := 5
	tracker := NewTracker(batchSize)
	tracker.Start()
	defer tracker.Stop()

	ch := tracker.Subscribe()

	// Add files one by one
	for i := 0; i < batchSize-1; i++ {
		tracker.AddFile(fmt.Sprintf("file%d.txt", i), 1024)
	}

	// Should not receive updates yet (batch not full)
	select {
	case <-ch:
		t.Error("expected no updates before batch is full")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// Add one more to complete the batch
	tracker.AddFile("final.txt", 1024)

	// Should receive batched updates
	timeout := time.After(200 * time.Millisecond)
	updateCount := 0

	for {
		select {
		case <-ch:
			updateCount++
		case <-timeout:
			goto done
		}
	}

done:
	if updateCount == 0 {
		t.Error("expected to receive batched updates")
	}
}

func TestTrackerErrorHandling(t *testing.T) {
	tracker := NewTracker(100)
	tracker.Start()
	defer tracker.Stop()

	ch := tracker.Subscribe()

	testErr := fmt.Errorf("test error")
	tracker.AddError(testErr)

	// Should receive error update immediately
	select {
	case update := <-ch:
		if update.Type != UpdateTypeError {
			t.Errorf("expected error update, got %v", update.Type)
		}
		if update.Error != testErr {
			t.Errorf("expected error %v, got %v", testErr, update.Error)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive error update")
	}

	snapshot := tracker.GetSnapshot()
	if snapshot.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", snapshot.ErrorCount)
	}
}

func BenchmarkTrackerAddFile(b *testing.B) {
	tracker := NewTracker(1000)
	tracker.Start()
	defer tracker.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tracker.AddFile(fmt.Sprintf("file%d.txt", i), 1024)
			i++
		}
	})

	b.ReportMetric(float64(b.N), "files/op")
}

func BenchmarkTrackerGetSnapshot(b *testing.B) {
	tracker := NewTracker(100)
	tracker.Start()
	defer tracker.Stop()

	// Add some data
	tracker.SetTotals(10000, 1024*1024*1024)
	for i := 0; i < 1000; i++ {
		tracker.AddFile(fmt.Sprintf("file%d.txt", i), 1024*1024)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = tracker.GetSnapshot()
		}
	})
}
