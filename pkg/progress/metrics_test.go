/**
 * Progress Metrics Tests
 * Test suite for metrics collection and performance calculations
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

package progress

import (
	"fmt"
	"testing"
	"time"
)

func TestNewMetricsCollector(t *testing.T) {
	t.Run("create with default settings", func(t *testing.T) {
		mc := NewMetricsCollector(0, 0)

		if mc.windowSize != 30*time.Second {
			t.Errorf("expected default window size 30s, got %v", mc.windowSize)
		}

		if mc.sampleInterval != 100*time.Millisecond {
			t.Errorf("expected default sample interval 100ms, got %v",
				mc.sampleInterval)
		}
	})

	t.Run("create with custom settings", func(t *testing.T) {
		mc := NewMetricsCollector(10*time.Second, 50*time.Millisecond)

		if mc.windowSize != 10*time.Second {
			t.Errorf("expected window size 10s, got %v", mc.windowSize)
		}
	})
}

func TestCircularBuffer(t *testing.T) {
	buffer := NewCircularBuffer(5)

	t.Run("add samples", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			buffer.Add(Sample{
				Timestamp: time.Now(),
				Bytes:     int64(i * 1024),
				Files:     int64(i),
			})
		}

		samples := buffer.GetAll()
		if len(samples) != 3 {
			t.Errorf("expected 3 samples, got %d", len(samples))
		}
	})

	t.Run("buffer overflow", func(t *testing.T) {
		// Fill buffer beyond capacity
		for i := 0; i < 10; i++ {
			buffer.Add(Sample{
				Timestamp: time.Now(),
				Bytes:     int64(i * 1024),
				Files:     int64(i),
			})
		}

		samples := buffer.GetAll()
		if len(samples) != 5 {
			t.Errorf("expected 5 samples (buffer size), got %d", len(samples))
		}

		// Verify we have the most recent samples
		lastSample := samples[len(samples)-1]
		if lastSample.Bytes != 9*1024 {
			t.Errorf("expected last sample bytes 9216, got %d", lastSample.Bytes)
		}
	})

	t.Run("get recent samples", func(t *testing.T) {
		buffer.Clear()

		now := time.Now()
		// Add samples with different timestamps
		for i := 0; i < 5; i++ {
			buffer.Add(Sample{
				Timestamp: now.Add(time.Duration(i) * time.Second),
				Bytes:     int64(i * 1024),
				Files:     int64(i),
			})
		}

		// Get samples from last 2 seconds
		recent := buffer.GetRecent(2 * time.Second)
		if len(recent) < 2 {
			t.Errorf("expected at least 2 recent samples, got %d", len(recent))
		}
	})
}

func TestMetricsAddSample(t *testing.T) {
	mc := NewMetricsCollector(30*time.Second, 100*time.Millisecond)

	// Add multiple samples
	for i := 0; i < 5; i++ {
		mc.AddSample(1024, 1)
		time.Sleep(10 * time.Millisecond)
	}

	if mc.totalBytes != 5*1024 {
		t.Errorf("expected total bytes 5120, got %d", mc.totalBytes)
	}

	if mc.totalFiles != 5 {
		t.Errorf("expected total files 5, got %d", mc.totalFiles)
	}
}

func TestFileMetrics(t *testing.T) {
	mc := NewMetricsCollector(30*time.Second, 100*time.Millisecond)

	t.Run("track file lifecycle", func(t *testing.T) {
		filename := "test.txt"
		fileSize := int64(1024 * 1024)

		// Start file
		mc.StartFile(filename, fileSize)

		metric, exists := mc.GetFileMetrics(filename)
		if !exists {
			t.Fatal("expected file metric to exist")
		}

		if metric.Size != fileSize {
			t.Errorf("expected file size %d, got %d", fileSize, metric.Size)
		}

		// Update progress
		mc.UpdateFile(filename, fileSize/2)

		metric, _ = mc.GetFileMetrics(filename)
		if metric.BytesTransferred != fileSize/2 {
			t.Errorf("expected bytes transferred %d, got %d",
				fileSize/2, metric.BytesTransferred)
		}

		// Complete file
		mc.CompleteFile(filename)

		metric, _ = mc.GetFileMetrics(filename)
		if metric.EndTime.IsZero() {
			t.Error("expected end time to be set")
		}
	})

	t.Run("track file errors", func(t *testing.T) {
		filename := "error.txt"
		mc.StartFile(filename, 1024)

		testErr := fmt.Errorf("download failed")
		mc.ErrorFile(filename, testErr)

		metric, _ := mc.GetFileMetrics(filename)
		if metric.Error != testErr {
			t.Errorf("expected error %v, got %v", testErr, metric.Error)
		}

		if metric.Retries != 1 {
			t.Errorf("expected 1 retry, got %d", metric.Retries)
		}
	})
}

func TestSpeedCalculations(t *testing.T) {
	mc := NewMetricsCollector(5*time.Second, 100*time.Millisecond)

	// Simulate steady transfer
	bytesPerSample := int64(1024 * 100) // 100KB per sample
	numSamples := 10

	for i := 0; i < numSamples; i++ {
		mc.AddSample(bytesPerSample, 1)
		time.Sleep(100 * time.Millisecond)
	}

	t.Run("current speed", func(t *testing.T) {
		speed := mc.GetCurrentSpeed()
		if speed <= 0 {
			t.Error("expected positive current speed")
		}

		// Speed should be approximately bytesPerSample / 0.1s = 1MB/s
		expectedSpeed := float64(bytesPerSample) / 0.1
		tolerance := expectedSpeed * 0.5 // 50% tolerance

		if speed < expectedSpeed-tolerance || speed > expectedSpeed+tolerance {
			t.Errorf("expected speed around %.0f B/s, got %.0f B/s",
				expectedSpeed, speed)
		}
	})

	t.Run("average speed", func(t *testing.T) {
		avgSpeed := mc.GetAverageSpeed()
		if avgSpeed <= 0 {
			t.Error("expected positive average speed")
		}
	})
}

func TestStats(t *testing.T) {
	mc := NewMetricsCollector(30*time.Second, 100*time.Millisecond)

	// Create some file activities
	mc.StartFile("file1.txt", 1024*1024)
	mc.CompleteFile("file1.txt")

	mc.StartFile("file2.txt", 2*1024*1024)
	mc.UpdateFile("file2.txt", 1024*1024)

	mc.StartFile("file3.txt", 1024*1024)
	mc.ErrorFile("file3.txt", fmt.Errorf("failed"))

	stats := mc.GetStats()

	t.Run("file counts", func(t *testing.T) {
		if stats.CompletedFiles != 1 {
			t.Errorf("expected 1 completed file, got %d", stats.CompletedFiles)
		}

		if stats.ActiveFiles != 1 {
			t.Errorf("expected 1 active file, got %d", stats.ActiveFiles)
		}

		if stats.FailedFiles != 1 {
			t.Errorf("expected 1 failed file, got %d", stats.FailedFiles)
		}
	})

	t.Run("ETA calculation", func(t *testing.T) {
		// Set up a scenario with known values
		mc2 := NewMetricsCollector(30*time.Second, 100*time.Millisecond)
		mc2.totalBytes = 1024 * 1024 * 100 // 100MB total
		mc2.AddSample(1024*1024*10, 10)    // 10MB completed

		time.Sleep(1 * time.Second)
		mc2.AddSample(1024*1024*10, 10) // Another 10MB (20MB total)

		stats2 := mc2.GetStats()
		stats2.TotalBytes = mc2.totalBytes
		stats2.CompletedBytes = 1024 * 1024 * 20
		stats2.CurrentSpeed = 1024 * 1024 * 10 // 10MB/s

		eta := stats2.EstimatedTimeRemaining()
		// Should be (100-20)MB / 10MB/s = 8 seconds
		expectedETA := 8 * time.Second

		// Allow 2 second tolerance for timing variations
		if eta < expectedETA-2*time.Second || eta > expectedETA+2*time.Second {
			t.Errorf("expected ETA around %v, got %v", expectedETA, eta)
		}
	})

	t.Run("percent complete", func(t *testing.T) {
		stats.TotalBytes = 1000
		stats.CompletedBytes = 250

		percent := stats.PercentComplete()
		if percent != 25.0 {
			t.Errorf("expected 25%%, got %.1f%%", percent)
		}
	})
}

func BenchmarkMetricsAddSample(b *testing.B) {
	mc := NewMetricsCollector(30*time.Second, 100*time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mc.AddSample(1024, 1)
		}
	})
}

func BenchmarkCircularBuffer(b *testing.B) {
	buffer := NewCircularBuffer(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.Add(Sample{
			Timestamp: time.Now(),
			Bytes:     int64(i * 1024),
			Files:     int64(i),
		})
	}
}
