/**
 * Progress Metrics
 * Performance metrics collection with moving averages
 * 
 * Features:
 * - Moving average calculation for transfer speeds
 * - Window-based sampling for accurate speed measurements
 * - Memory-efficient circular buffer implementation
 * - Per-file and overall performance metrics
 * - Thread-safe metric collection
 * 
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

package progress

import (
  "sync"
  "time"
)

// MetricsCollector collects performance metrics during sync operations
type MetricsCollector struct {
  mu              sync.RWMutex
  samples         *CircularBuffer
  fileMetrics     map[string]*FileMetric
  startTime       time.Time
  totalBytes      int64
  totalFiles      int64
  windowSize      time.Duration
  sampleInterval  time.Duration
}

// FileMetric tracks metrics for individual files
type FileMetric struct {
  FileName    string
  Size        int64
  StartTime   time.Time
  EndTime     time.Time
  BytesTransferred int64
  Retries     int
  Error       error
}

// Sample represents a point-in-time measurement
type Sample struct {
  Timestamp time.Time
  Bytes     int64
  Files     int64
}

// CircularBuffer implements a fixed-size circular buffer for samples
type CircularBuffer struct {
  buffer   []Sample
  size     int
  head     int
  tail     int
  count    int
  mu       sync.RWMutex
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(windowSize, sampleInterval time.Duration) *MetricsCollector {
  if windowSize == 0 {
    windowSize = 30 * time.Second
  }
  if sampleInterval == 0 {
    sampleInterval = 100 * time.Millisecond
  }
  
  // Calculate buffer size based on window and sample interval
  bufferSize := int(windowSize / sampleInterval)
  if bufferSize < 10 {
    bufferSize = 10
  }
  
  return &MetricsCollector{
    samples:        NewCircularBuffer(bufferSize),
    fileMetrics:    make(map[string]*FileMetric),
    windowSize:     windowSize,
    sampleInterval: sampleInterval,
    startTime:      time.Now(),
  }
}

// NewCircularBuffer creates a new circular buffer
func NewCircularBuffer(size int) *CircularBuffer {
  return &CircularBuffer{
    buffer: make([]Sample, size),
    size:   size,
  }
}

// AddSample adds a new sample to the metrics collector
func (mc *MetricsCollector) AddSample(bytes, files int64) {
  mc.mu.Lock()
  defer mc.mu.Unlock()
  
  mc.totalBytes += bytes
  mc.totalFiles += files
  
  sample := Sample{
    Timestamp: time.Now(),
    Bytes:     mc.totalBytes,
    Files:     mc.totalFiles,
  }
  
  mc.samples.Add(sample)
}

// StartFile begins tracking metrics for a file
func (mc *MetricsCollector) StartFile(filename string, size int64) {
  mc.mu.Lock()
  defer mc.mu.Unlock()
  
  mc.fileMetrics[filename] = &FileMetric{
    FileName:  filename,
    Size:      size,
    StartTime: time.Now(),
  }
}

// UpdateFile updates progress for a file
func (mc *MetricsCollector) UpdateFile(filename string, bytesTransferred int64) {
  mc.mu.Lock()
  defer mc.mu.Unlock()
  
  if metric, exists := mc.fileMetrics[filename]; exists {
    metric.BytesTransferred = bytesTransferred
  }
}

// CompleteFile marks a file as completed
func (mc *MetricsCollector) CompleteFile(filename string) {
  mc.mu.Lock()
  defer mc.mu.Unlock()
  
  if metric, exists := mc.fileMetrics[filename]; exists {
    metric.EndTime = time.Now()
  }
}

// ErrorFile marks a file as having an error
func (mc *MetricsCollector) ErrorFile(filename string, err error) {
  mc.mu.Lock()
  defer mc.mu.Unlock()
  
  if metric, exists := mc.fileMetrics[filename]; exists {
    metric.Error = err
    metric.Retries++
  }
}

// GetCurrentSpeed calculates the current transfer speed using moving average
// Uses exponential weighted moving average for smooth speed calculations
func (mc *MetricsCollector) GetCurrentSpeed() float64 {
  mc.mu.RLock()
  defer mc.mu.RUnlock()
  
  samples := mc.samples.GetRecent(mc.windowSize)
  if len(samples) < 2 {
    return 0
  }
  
  // Calculate speed using weighted average, giving more weight to recent samples
  var weightedSpeed float64
  var totalWeight float64
  
  for i := 1; i < len(samples); i++ {
    prev := samples[i-1]
    curr := samples[i]
    
    duration := curr.Timestamp.Sub(prev.Timestamp).Seconds()
    if duration <= 0 {
      continue
    }
    
    byteDiff := curr.Bytes - prev.Bytes
    speed := float64(byteDiff) / duration
    
    // Apply exponential weight (more recent = higher weight)
    weight := float64(i) / float64(len(samples)-1)
    weight = weight * weight // Square for exponential weighting
    
    weightedSpeed += speed * weight
    totalWeight += weight
  }
  
  if totalWeight == 0 {
    return 0
  }
  
  return weightedSpeed / totalWeight
}

// GetAverageSpeed calculates the overall average speed
func (mc *MetricsCollector) GetAverageSpeed() float64 {
  mc.mu.RLock()
  defer mc.mu.RUnlock()
  
  elapsed := time.Since(mc.startTime).Seconds()
  if elapsed <= 0 {
    return 0
  }
  
  return float64(mc.totalBytes) / elapsed
}

// GetFileMetrics returns metrics for a specific file
func (mc *MetricsCollector) GetFileMetrics(filename string) (*FileMetric, bool) {
  mc.mu.RLock()
  defer mc.mu.RUnlock()
  
  metric, exists := mc.fileMetrics[filename]
  if !exists {
    return nil, false
  }
  
  // Return a copy to avoid race conditions
  metricCopy := *metric
  return &metricCopy, true
}

// GetStats returns overall statistics
func (mc *MetricsCollector) GetStats() Stats {
  mc.mu.RLock()
  defer mc.mu.RUnlock()
  
  stats := Stats{
    TotalBytes:    mc.totalBytes,
    TotalFiles:    mc.totalFiles,
    ElapsedTime:   time.Since(mc.startTime),
    CurrentSpeed:  mc.GetCurrentSpeed(),
    AverageSpeed:  mc.GetAverageSpeed(),
  }
  
  // Calculate file statistics
  for _, metric := range mc.fileMetrics {
    if metric.Error != nil {
      stats.FailedFiles++
    } else if !metric.EndTime.IsZero() {
      stats.CompletedFiles++
      stats.CompletedBytes += metric.BytesTransferred
    } else {
      stats.ActiveFiles++
    }
    
    if metric.Retries > 0 {
      stats.TotalRetries += metric.Retries
    }
  }
  
  return stats
}

// Add adds a sample to the circular buffer
func (cb *CircularBuffer) Add(sample Sample) {
  cb.mu.Lock()
  defer cb.mu.Unlock()
  
  cb.buffer[cb.head] = sample
  cb.head = (cb.head + 1) % cb.size
  
  if cb.count < cb.size {
    cb.count++
  } else {
    cb.tail = (cb.tail + 1) % cb.size
  }
}

// GetRecent returns samples from the last duration
func (cb *CircularBuffer) GetRecent(duration time.Duration) []Sample {
  cb.mu.RLock()
  defer cb.mu.RUnlock()
  
  if cb.count == 0 {
    return nil
  }
  
  cutoff := time.Now().Add(-duration)
  samples := make([]Sample, 0, cb.count)
  
  // Iterate through the buffer
  for i := 0; i < cb.count; i++ {
    index := (cb.tail + i) % cb.size
    sample := cb.buffer[index]
    
    if sample.Timestamp.After(cutoff) {
      samples = append(samples, sample)
    }
  }
  
  return samples
}

// GetAll returns all samples in chronological order
func (cb *CircularBuffer) GetAll() []Sample {
  cb.mu.RLock()
  defer cb.mu.RUnlock()
  
  if cb.count == 0 {
    return nil
  }
  
  samples := make([]Sample, cb.count)
  for i := 0; i < cb.count; i++ {
    index := (cb.tail + i) % cb.size
    samples[i] = cb.buffer[index]
  }
  
  return samples
}

// Clear clears the buffer
func (cb *CircularBuffer) Clear() {
  cb.mu.Lock()
  defer cb.mu.Unlock()
  
  cb.head = 0
  cb.tail = 0
  cb.count = 0
}

// Stats represents overall sync statistics
type Stats struct {
  TotalBytes     int64
  TotalFiles     int64
  CompletedBytes int64
  CompletedFiles int64
  FailedFiles    int64
  ActiveFiles    int64
  TotalRetries   int
  ElapsedTime    time.Duration
  CurrentSpeed   float64
  AverageSpeed   float64
}

// EstimatedTimeRemaining calculates ETA based on current speed
func (s Stats) EstimatedTimeRemaining() time.Duration {
  if s.CurrentSpeed <= 0 || s.CompletedBytes >= s.TotalBytes {
    return 0
  }
  
  remainingBytes := s.TotalBytes - s.CompletedBytes
  seconds := float64(remainingBytes) / s.CurrentSpeed
  return time.Duration(seconds) * time.Second
}

// PercentComplete returns completion percentage
func (s Stats) PercentComplete() float64 {
  if s.TotalBytes == 0 {
    if s.TotalFiles == 0 {
      return 0
    }
    return float64(s.CompletedFiles) / float64(s.TotalFiles) * 100
  }
  return float64(s.CompletedBytes) / float64(s.TotalBytes) * 100
}