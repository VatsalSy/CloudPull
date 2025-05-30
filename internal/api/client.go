package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"github.com/VatsalSy/CloudPull/internal/errors"
	"github.com/VatsalSy/CloudPull/internal/logger"
)

/**
 * Google Drive API Client Wrapper
 *
 * Features:
 * - High-level API for common Drive operations
 * - Automatic retry with exponential backoff
 * - Rate limiting integration
 * - Resumable downloads with byte ranges
 * - Export functionality for Google Workspace files
 * - Comprehensive error handling
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

const (
	// Default page size for listing files
	defaultPageSize = 1000

	// Maximum number of retries for API calls
	maxRetries = 3

	// Base delay for exponential backoff
	baseRetryDelay = time.Second

	// Default chunk size for downloads (10MB)
	defaultChunkSize = 10 * 1024 * 1024
)

// Google Workspace MIME type mappings.
var googleMimeTypes = map[string]string{
	"application/vnd.google-apps.document":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"application/vnd.google-apps.spreadsheet":  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"application/vnd.google-apps.presentation": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"application/vnd.google-apps.drawing":      "application/pdf",
	"application/vnd.google-apps.form":         "application/pdf",
}

// File extensions for export formats.
var exportExtensions = map[string]string{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"application/pdf": ".pdf",
}

// DriveClient provides high-level operations for Google Drive API.
type DriveClient struct {
	service     *drive.Service
	rateLimiter *RateLimiter
	logger      *logger.Logger
	chunkSize   int64
}

// NewDriveClient creates a new Drive API client.
func NewDriveClient(service *drive.Service, rateLimiter *RateLimiter, logger *logger.Logger) *DriveClient {
	return &DriveClient{
		service:     service,
		rateLimiter: rateLimiter,
		logger:      logger,
		chunkSize:   defaultChunkSize,
	}
}

// FileInfo contains essential file metadata.
type FileInfo struct {
	ModifiedTime time.Time
	ID           string
	Name         string
	MimeType     string
	MD5Checksum  string
	ExportFormat string
	Parents      []string
	Size         int64
	IsFolder     bool
	CanExport    bool
}

// ListFiles lists files in a folder with pagination.
func (dc *DriveClient) ListFiles(ctx context.Context, folderID string, pageToken string) ([]*FileInfo, string, error) {
	dc.logger.Debug("ListFiles called", "folderID", folderID, "pageToken", pageToken)

	// Wait for rate limit
	if err := dc.rateLimiter.Wait(ctx); err != nil {
		dc.logger.Error(err, "Rate limiter error")
		return nil, "", err
	}

	query := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	dc.logger.Debug("Constructed query", "query", query)

	call := dc.service.Files.List().
		Q(query).
		PageSize(int64(defaultPageSize)).
		Fields("nextPageToken, files(id, name, mimeType, size, md5Checksum, modifiedTime, parents)").
		OrderBy("folder,name")

	if pageToken != "" {
		call = call.PageToken(pageToken)
	}

	dc.logger.Debug("Executing API call")
	var fileList *drive.FileList
	err := dc.retryWithBackoff(ctx, func() error {
		var err error
		fileList, err = call.Do()
		if err != nil {
			dc.logger.Error(err, "API call failed")
		}
		return err
	})

	if err != nil {
		dc.logger.Error(err, "Failed to list files after retries")
		return nil, "", errors.Wrap(err, "failed to list files")
	}
	dc.logger.Debug("API call successful", "fileCount", len(fileList.Files))

	files := make([]*FileInfo, 0, len(fileList.Files))
	for _, f := range fileList.Files {
		files = append(files, dc.convertFileInfo(f))
	}

	return files, fileList.NextPageToken, nil
}

// GetFile retrieves file metadata.
func (dc *DriveClient) GetFile(ctx context.Context, fileID string) (*FileInfo, error) {
	// Wait for rate limit
	if err := dc.rateLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	var file *drive.File
	err := dc.retryWithBackoff(ctx, func() error {
		var err error
		file, err = dc.service.Files.Get(fileID).
			Fields("id, name, mimeType, size, md5Checksum, modifiedTime, parents").
			Do()
		return err
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to get file metadata")
	}

	return dc.convertFileInfo(file), nil
}

// DownloadFile downloads a file with resumable support.
func (dc *DriveClient) DownloadFile(ctx context.Context, fileID string, destPath string, progressFn func(downloaded, total int64)) error {
	// Get file metadata first
	fileInfo, err := dc.GetFile(ctx, fileID)
	if err != nil {
		return err
	}

	// Check if it's a Google Workspace file
	if fileInfo.CanExport {
		return dc.ExportFile(ctx, fileID, fileInfo.ExportFormat, destPath, progressFn)
	}

	// Regular file download
	return dc.downloadRegularFile(ctx, fileID, destPath, fileInfo.Size, progressFn)
}

// downloadRegularFile handles downloading of regular (non-Google Workspace) files.
func (dc *DriveClient) downloadRegularFile(ctx context.Context, fileID string, destPath string, fileSize int64, progressFn func(downloaded, total int64)) error {
	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return errors.Wrap(err, "failed to create destination directory")
	}

	// Open/create destination file
	file, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create destination file")
	}
	defer file.Close()

	// Check if file exists and get current size for resume
	stat, err := file.Stat()
	if err != nil {
		return errors.Wrap(err, "failed to stat destination file")
	}

	startOffset := stat.Size()
	if startOffset >= fileSize {
		dc.logger.Info("File already downloaded", "file", destPath)
		if progressFn != nil {
			progressFn(fileSize, fileSize)
		}
		return nil
	}

	// Seek to end for append
	if _, err := file.Seek(startOffset, 0); err != nil {
		return errors.Wrap(err, "failed to seek in file")
	}

	// Download in chunks
	for startOffset < fileSize {
		endOffset := startOffset + dc.chunkSize - 1
		if endOffset >= fileSize {
			endOffset = fileSize - 1
		}

		dc.logger.Debug("Downloading chunk",
			"fileID", fileID,
			"start", startOffset,
			"end", endOffset)

		// Wait for rate limit
		if err := dc.rateLimiter.Wait(ctx); err != nil {
			return err
		}

		// Download chunk with retries
		var resp *http.Response
		err := dc.retryWithBackoff(ctx, func() error {
			req := dc.service.Files.Get(fileID)
			req = req.AcknowledgeAbuse(true) // Handle potential abuse warnings
			req.Header().Set("Range", fmt.Sprintf("bytes=%d-%d", startOffset, endOffset))

			var err error
			resp, err = req.Download()
			return err
		})

		if err != nil {
			return errors.Wrap(err, "failed to download chunk")
		}

		// Write chunk to file
		written, err := io.Copy(file, resp.Body)
		resp.Body.Close()

		if err != nil {
			return errors.Wrap(err, "failed to write chunk")
		}

		startOffset += written

		if progressFn != nil {
			progressFn(startOffset, fileSize)
		}
	}

	dc.logger.Info("File downloaded successfully", "file", destPath)
	return nil
}

// ExportFile exports a Google Workspace file.
func (dc *DriveClient) ExportFile(ctx context.Context, fileID string, mimeType string, destPath string, progressFn func(downloaded, total int64)) error {
	// Determine export format
	exportMimeType := mimeType
	if exportMimeType == "" {
		// Get file metadata to determine type
		fileInfo, err := dc.GetFile(ctx, fileID)
		if err != nil {
			return err
		}

		exportMimeType = googleMimeTypes[fileInfo.MimeType]
		if exportMimeType == "" {
			return errors.Errorf("unsupported Google Workspace file type: %s", fileInfo.MimeType)
		}
	}

	// Add appropriate extension if not present
	if ext := exportExtensions[exportMimeType]; ext != "" {
		if !strings.HasSuffix(destPath, ext) {
			destPath += ext
		}
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return errors.Wrap(err, "failed to create destination directory")
	}

	// Wait for rate limit
	if err := dc.rateLimiter.Wait(ctx); err != nil {
		return err
	}

	// Export file with retries
	var resp *http.Response
	err := dc.retryWithBackoff(ctx, func() error {
		var err error
		resp, err = dc.service.Files.Export(fileID, exportMimeType).Download()
		return err
	})

	if err != nil {
		return errors.Wrap(err, "failed to export file")
	}
	defer resp.Body.Close()

	// Create destination file
	file, err := os.Create(destPath)
	if err != nil {
		return errors.Wrap(err, "failed to create destination file")
	}
	defer file.Close()

	// Copy content with progress tracking
	var written int64
	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := file.Write(buf[:n]); writeErr != nil {
				return errors.Wrap(writeErr, "failed to write to file")
			}
			written += int64(n)

			if progressFn != nil {
				// For exports, we don't know total size in advance
				progressFn(written, -1)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to read export data")
		}
	}

	dc.logger.Info("File exported successfully",
		"file", destPath,
		"format", exportMimeType,
		"size", written)

	return nil
}

// GetRootFolderID returns the ID of the root folder.
func (dc *DriveClient) GetRootFolderID() string {
	return "root"
}

// convertFileInfo converts Drive API file to FileInfo.
func (dc *DriveClient) convertFileInfo(f *drive.File) *FileInfo {
	info := &FileInfo{
		ID:          f.Id,
		Name:        f.Name,
		MimeType:    f.MimeType,
		Size:        f.Size,
		MD5Checksum: f.Md5Checksum,
		Parents:     f.Parents,
		IsFolder:    f.MimeType == "application/vnd.google-apps.folder",
	}

	// Parse modified time
	if f.ModifiedTime != "" {
		if t, err := time.Parse(time.RFC3339, f.ModifiedTime); err == nil {
			info.ModifiedTime = t
		}
	}

	// Check if it's a Google Workspace file that needs export
	if exportFormat, ok := googleMimeTypes[f.MimeType]; ok {
		info.CanExport = true
		info.ExportFormat = exportFormat
	}

	return info
}

// retryWithBackoff implements exponential backoff retry logic.
func (dc *DriveClient) retryWithBackoff(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !dc.isRetryableError(err) {
			return err
		}

		// Calculate backoff delay
		delay := baseRetryDelay * time.Duration(1<<uint(attempt))

		// Add jitter (±25%)
		jitter := time.Duration(float64(delay) * 0.25 * (2*generateRandom() - 1))
		delay += jitter

		dc.logger.Warn("API call failed, retrying",
			"attempt", attempt+1,
			"delay", delay,
			"error", err)

		// Wait with context
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return errors.Wrap(lastErr, "max retries exceeded")
}

// isRetryableError checks if an error is retryable.
func (dc *DriveClient) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for Google API errors
	if apiErr, ok := err.(*googleapi.Error); ok {
		switch apiErr.Code {
		case 429, 500, 502, 503, 504: // Rate limit and server errors
			return true
		case 403: // Check for rate limit in 403 errors
			for _, e := range apiErr.Errors {
				if e.Reason == "userRateLimitExceeded" || e.Reason == "rateLimitExceeded" {
					return true
				}
			}
		}
	}

	// Check for network errors
	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") {

		return true
	}

	return false
}

// GetFileContent downloads a file chunk with byte range support.
func (dc *DriveClient) GetFileContent(ctx context.Context, fileID string, startOffset, endOffset int64) (*http.Response, error) {
	// Wait for rate limit
	if err := dc.rateLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	// Create request with byte range
	req := dc.service.Files.Get(fileID)
	req = req.AcknowledgeAbuse(true)
	req.Header().Set("Range", fmt.Sprintf("bytes=%d-%d", startOffset, endOffset))

	var resp *http.Response
	err := dc.retryWithBackoff(ctx, func() error {
		var err error
		resp, err = req.Download()
		return err
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to download file content")
	}

	return resp, nil
}

// generateRandom generates a random float between 0 and 1.
func generateRandom() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}
