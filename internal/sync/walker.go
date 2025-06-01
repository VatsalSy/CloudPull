/**
 * Memory-Efficient Folder Tree Walker for CloudPull Sync Engine
 *
 * Features:
 * - Streaming folder traversal without loading entire tree
 * - Support for BFS and DFS traversal strategies
 * - Pagination support for large folders (1000 items per page)
 * - Folder filtering patterns
 * - Google Drive shortcuts handling
 * - Progress reporting during traversal
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/VatsalSy/CloudPull/internal/api"
	"github.com/VatsalSy/CloudPull/internal/errors"
	"github.com/VatsalSy/CloudPull/internal/logger"
	"github.com/VatsalSy/CloudPull/internal/state"
)

// TraversalStrategy defines the folder traversal strategy.
type TraversalStrategy int

const (
	// TraversalBFS performs breadth-first search traversal.
	TraversalBFS TraversalStrategy = iota

	// TraversalDFS performs depth-first search traversal.
	TraversalDFS
)

// WalkerConfig contains configuration for the folder walker.
type WalkerConfig struct {
	IncludePatterns   []string
	ExcludePatterns   []string
	Strategy          TraversalStrategy
	MaxDepth          int
	Concurrency       int
	ChannelBufferSize int
	FollowShortcuts   bool
}

// DefaultWalkerConfig returns default walker configuration.
func DefaultWalkerConfig() *WalkerConfig {
	return &WalkerConfig{
		Strategy:          TraversalBFS,
		MaxDepth:          0, // unlimited
		FollowShortcuts:   false,
		Concurrency:       3,
		ChannelBufferSize: 100,
	}
}

// FolderWalker implements efficient folder tree traversal.
type FolderWalker struct {
	ctx             context.Context
	cancel          context.CancelFunc
	config          *WalkerConfig
	stateManager    *state.Manager
	progressTracker *ProgressTracker
	logger          *logger.Logger
	client          *api.DriveClient
	excludeRegexps  []*regexp.Regexp
	includeRegexps  []*regexp.Regexp
	errors          []error
	wg              sync.WaitGroup
	foldersScanned  int64
	filesFound      int64
	totalSize       int64
	mu              sync.RWMutex
}

// WalkResult represents a folder walk result.
type WalkResult struct {
	Error      error
	Folder     *state.Folder
	SkipReason string
	Files      []*state.File
	Depth      int
	IsSkipped  bool
}

// NewFolderWalker creates a new folder walker.
func NewFolderWalker(
	client *api.DriveClient,
	stateManager *state.Manager,
	progressTracker *ProgressTracker,
	logger *logger.Logger,
	config *WalkerConfig,
) (*FolderWalker, error) {

	if config == nil {
		config = DefaultWalkerConfig()
	}

	walker := &FolderWalker{
		config:          config,
		client:          client,
		stateManager:    stateManager,
		progressTracker: progressTracker,
		logger:          logger,
	}

	// Compile include patterns
	if len(config.IncludePatterns) > 0 {
		walker.includeRegexps = make([]*regexp.Regexp, 0, len(config.IncludePatterns))
		for _, pattern := range config.IncludePatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("invalid include pattern: %s", pattern))
			}
			walker.includeRegexps = append(walker.includeRegexps, re)
		}
	}

	// Compile exclude patterns
	if len(config.ExcludePatterns) > 0 {
		walker.excludeRegexps = make([]*regexp.Regexp, 0, len(config.ExcludePatterns))
		for _, pattern := range config.ExcludePatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("invalid exclude pattern: %s", pattern))
			}
			walker.excludeRegexps = append(walker.excludeRegexps, re)
		}
	}

	return walker, nil
}

// Walk starts walking the folder tree from the given root.
func (fw *FolderWalker) Walk(ctx context.Context, rootFolderID string, sessionID string) (<-chan *WalkResult, error) {
	fw.logger.Debug("Walk called", "rootFolderID", rootFolderID, "sessionID", sessionID, "strategy", fw.config.Strategy)

	// Create cancellable context
	fw.ctx, fw.cancel = context.WithCancel(ctx)

	// Create result channel
	resultChan := make(chan *WalkResult, fw.config.ChannelBufferSize)

	// Start walking based on strategy
	switch fw.config.Strategy {
	case TraversalBFS:
		fw.logger.Debug("Starting BFS traversal")
		fw.wg.Add(1)
		go fw.walkBFS(rootFolderID, sessionID, resultChan)
	case TraversalDFS:
		fw.logger.Debug("Starting DFS traversal")
		fw.wg.Add(1)
		go fw.walkDFS(rootFolderID, sessionID, "", 0, resultChan)
	default:
		close(resultChan)
		return nil, fmt.Errorf("unknown traversal strategy: %v", fw.config.Strategy)
	}

	// Start channel closer
	go func() {
		fw.wg.Wait()
		close(resultChan)
	}()

	fw.logger.Debug("Walk started successfully")
	return resultChan, nil
}

// Stop stops the folder walker.
func (fw *FolderWalker) Stop() {
	if fw.cancel != nil {
		fw.cancel()
	}
	fw.wg.Wait()
}

// GetStats returns walker statistics.
func (fw *FolderWalker) GetStats() *WalkerStats {
	fw.mu.RLock()
	defer fw.mu.RUnlock()

	return &WalkerStats{
		FoldersScanned: fw.foldersScanned,
		FilesFound:     fw.filesFound,
		TotalSize:      fw.totalSize,
		ErrorCount:     len(fw.errors),
	}
}

// walkBFS performs breadth-first search traversal.
func (fw *FolderWalker) walkBFS(rootFolderID string, sessionID string, resultChan chan<- *WalkResult) {
	defer fw.wg.Done()
	fw.logger.Debug("walkBFS started", "rootFolderID", rootFolderID, "sessionID", sessionID)

	type folderTask struct {
		folderID   string
		parentPath string
		depth      int
	}

	// Queue for BFS
	queue := make(chan *folderTask, fw.config.ChannelBufferSize)

	// Track active tasks
	var activeTasksWg sync.WaitGroup
	activeTasksWg.Add(1) // Start with 1 for the root folder

	// Start workers
	workers := fw.config.Concurrency
	workerWg := sync.WaitGroup{}
	fw.logger.Debug("Starting workers", "count", workers)

	for i := 0; i < workers; i++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()

			for task := range queue {
				if fw.ctx.Err() != nil {
					activeTasksWg.Done()
					return
				}

				// Process folder
				folder, files, subfolders, err := fw.processFolder(
					task.folderID,
					task.parentPath,
					sessionID,
					task.depth,
				)

				// Send result
				result := &WalkResult{
					Folder: folder,
					Files:  files,
					Error:  err,
					Depth:  task.depth,
				}

				select {
				case resultChan <- result:
				case <-fw.ctx.Done():
					activeTasksWg.Done()
					return
				}

				// Queue subfolders if within depth limit
				withinDepthLimit := fw.config.MaxDepth == -1 || fw.config.MaxDepth == 0 || task.depth < fw.config.MaxDepth
				fw.logger.Info("Checking subfolder queueing",
					"err", err,
					"subfolders_count", len(subfolders),
					"current_depth", task.depth,
					"max_depth", fw.config.MaxDepth,
					"depth_check", withinDepthLimit,
				)
				if err == nil && withinDepthLimit {
					fw.logger.Info("Queueing subfolders",
						"count", len(subfolders),
						"parent_folder", task.folderID,
						"current_depth", task.depth,
						"max_depth", fw.config.MaxDepth,
					)
					for _, subfolder := range subfolders {
						activeTasksWg.Add(1) // Add before queuing
						subTask := &folderTask{
							folderID:   subfolder.ID,
							parentPath: filepath.Join(task.parentPath, subfolder.Name),
							depth:      task.depth + 1,
						}

						fw.logger.Debug("Queueing subfolder task",
							"folder_id", subfolder.ID,
							"folder_name", subfolder.Name,
							"depth", subTask.depth,
						)

						select {
						case queue <- subTask:
						case <-fw.ctx.Done():
							activeTasksWg.Done()
							return
						}
					}
				}

				activeTasksWg.Done() // Mark this task as done
			}
		}(i)
	}

	// Start with root folder
	queue <- &folderTask{
		folderID:   rootFolderID,
		parentPath: "",
		depth:      0,
	}

	// Close queue when all tasks are done
	go func() {
		activeTasksWg.Wait()
		close(queue)
	}()

	// Wait for all workers
	workerWg.Wait()
}

// walkDFS performs depth-first search traversal.
func (fw *FolderWalker) walkDFS(
	folderID string,
	sessionID string,
	parentPath string,
	depth int,
	resultChan chan<- *WalkResult,
) {

	if depth == 0 {
		defer fw.wg.Done()
	}

	// Check context
	if fw.ctx.Err() != nil {
		return
	}

	// Check depth limit
	if fw.config.MaxDepth > 0 && depth > fw.config.MaxDepth {
		return
	}

	// Process folder
	folder, files, subfolders, err := fw.processFolder(folderID, parentPath, sessionID, depth)

	// Send result
	result := &WalkResult{
		Folder: folder,
		Files:  files,
		Error:  err,
		Depth:  depth,
	}

	select {
	case resultChan <- result:
	case <-fw.ctx.Done():
		return
	}

	// Recursively process subfolders
	if err == nil {
		for _, subfolder := range subfolders {
			fw.walkDFS(
				subfolder.ID,
				sessionID,
				filepath.Join(parentPath, subfolder.Name),
				depth+1,
				resultChan,
			)
		}
	}
}

// processFolder processes a single folder.
func (fw *FolderWalker) processFolder(
	folderID string,
	parentPath string,
	sessionID string,
	depth int,
) (*state.Folder, []*state.File, []*api.FileInfo, error) {

	fw.logger.Debug("processFolder called", "folderID", folderID, "parentPath", parentPath, "depth", depth)

	// Get folder metadata
	var folderName string

	if folderID == "root" {
		folderName = "root"
	} else {
		fw.logger.Debug("Getting folder metadata from API", "folderID", folderID)
		info, err := fw.client.GetFile(fw.ctx, folderID)
		if err != nil {
			fw.logger.Error(err, "Failed to get folder metadata", "folderID", folderID)
			fw.mu.Lock()
			fw.errors = append(fw.errors, err)
			fw.mu.Unlock()
			return nil, nil, nil, errors.Wrap(err, "failed to get folder metadata")
		}
		folderName = info.Name
		fw.logger.Debug("Got folder metadata", "folderName", folderName)
	}

	folderPath := filepath.Join(parentPath, folderName)

	// Check if folder should be skipped
	if fw.shouldSkipFolder(folderPath) {
		return nil, nil, nil, nil
	}

	// Create folder record
	folder := &state.Folder{
		ID:        generateID(),
		DriveID:   folderID,
		SessionID: sessionID,
		Name:      folderName,
		Path:      folderPath,
		Status:    state.FolderStatusScanning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save to database
	if err := fw.stateManager.CreateFolder(fw.ctx, folder); err != nil {
		fw.logger.Error(err, "Failed to create folder record",
			"folder_id", folderID,
			"folder_path", folderPath,
		)
	}

	// Notify progress tracker
	fw.progressTracker.FolderStarted(folder.ID, folder.Name, folder.Path)

	// List folder contents with pagination
	var allFiles []*state.File
	var subfolders []*api.FileInfo
	pageToken := ""
	pageCount := 0

	for {
		// Check context
		if fw.ctx.Err() != nil {
			return folder, allFiles, subfolders, fw.ctx.Err()
		}

		// List files
		files, nextPageToken, err := fw.client.ListFiles(fw.ctx, folderID, pageToken)
		if err != nil {
			folder.Status = state.FolderStatusFailed
			folder.ErrorMessage.Valid = true
			folder.ErrorMessage.String = err.Error()
			fw.stateManager.UpdateFolder(fw.ctx, folder)

			fw.mu.Lock()
			fw.errors = append(fw.errors, err)
			fw.mu.Unlock()

			return folder, allFiles, subfolders, errors.Wrap(err, "failed to list folder contents")
		}

		pageCount++
		fw.logger.Debug("Listed folder page",
			"folder_id", folderID,
			"page", pageCount,
			"items", len(files),
		)

		// Process files
		for _, fileInfo := range files {
			if fileInfo.IsFolder {
				// Handle shortcuts if configured
				if !fw.config.FollowShortcuts && fw.isShortcut(fileInfo) {
					fw.logger.Debug("Skipping shortcut folder",
						"folder_id", fileInfo.ID,
						"folder_name", fileInfo.Name,
					)
					continue
				}

				fw.logger.Info("Found subfolder",
					"folder_id", fileInfo.ID,
					"folder_name", fileInfo.Name,
					"parent_folder", folderName,
				)
				subfolders = append(subfolders, fileInfo)
			} else {
				// Create file record
				file := fw.createFileRecord(fileInfo, folder, sessionID, folderPath)
				allFiles = append(allFiles, file)

				// Update metrics
				fw.mu.Lock()
				fw.filesFound++
				fw.totalSize += file.Size
				fw.mu.Unlock()
			}
		}

		// Check if more pages
		if nextPageToken == "" {
			break
		}
		pageToken = nextPageToken
	}

	// Batch save files to database
	if len(allFiles) > 0 {
		if err := fw.stateManager.CreateFiles(fw.ctx, allFiles); err != nil {
			fw.logger.Error(err, "Failed to create file records",
				"folder_id", folderID,
				"file_count", len(allFiles),
			)
		}
	}

	// Update folder status
	folder.Status = state.FolderStatusScanned
	fw.stateManager.UpdateFolder(fw.ctx, folder)

	// Update metrics
	fw.mu.Lock()
	fw.foldersScanned++
	fw.mu.Unlock()

	// Notify progress tracker
	fw.progressTracker.FolderCompleted(folder.ID, folder.Name, folder.Path, int64(len(allFiles)))

	return folder, allFiles, subfolders, nil
}

// shouldSkipFolder checks if a folder should be skipped based on patterns.
func (fw *FolderWalker) shouldSkipFolder(folderPath string) bool {
	// Check exclude patterns
	for _, re := range fw.excludeRegexps {
		if re.MatchString(folderPath) {
			fw.logger.Debug("Skipping excluded folder",
				"path", folderPath,
				"pattern", re.String(),
			)
			return true
		}
	}

	// Check include patterns (if any are set)
	if len(fw.includeRegexps) > 0 {
		included := false
		for _, re := range fw.includeRegexps {
			if re.MatchString(folderPath) {
				included = true
				break
			}
		}
		if !included {
			fw.logger.Debug("Skipping non-included folder",
				"path", folderPath,
			)
			return true
		}
	}

	return false
}

// isShortcut checks if a file is a Google Drive shortcut.
func (fw *FolderWalker) isShortcut(fileInfo *api.FileInfo) bool {
	return fileInfo.MimeType == "application/vnd.google-apps.shortcut" ||
		strings.HasSuffix(fileInfo.MimeType, ".link")
}

// createFileRecord creates a file record from Drive API file info.
func (fw *FolderWalker) createFileRecord(
	fileInfo *api.FileInfo,
	folder *state.Folder,
	sessionID string,
	folderPath string,
) *state.File {

	fullPath := filepath.Join(folderPath, fileInfo.Name)

	fw.logger.Debug("Creating file record",
		"file_id", fileInfo.ID,
		"file_name", fileInfo.Name,
		"folder_path", folderPath,
		"full_path", fullPath,
		"size", fileInfo.Size,
		"mime_type", fileInfo.MimeType,
	)

	file := &state.File{
		ID:               generateID(),
		DriveID:          fileInfo.ID,
		FolderID:         folder.ID,
		SessionID:        sessionID,
		Name:             fileInfo.Name,
		Path:             fullPath,
		Size:             fileInfo.Size,
		Status:           state.FileStatusPending,
		BytesDownloaded:  0,
		DownloadAttempts: 0,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Set optional fields
	if fileInfo.MD5Checksum != "" {
		file.MD5Checksum.Valid = true
		file.MD5Checksum.String = fileInfo.MD5Checksum
	}

	if fileInfo.MimeType != "" {
		file.MimeType.Valid = true
		file.MimeType.String = fileInfo.MimeType
	}

	if fileInfo.CanExport {
		file.IsGoogleDoc = true
		file.ExportMimeType.Valid = true
		file.ExportMimeType.String = fileInfo.ExportFormat
	}

	if !fileInfo.ModifiedTime.IsZero() {
		file.DriveModifiedTime.Valid = true
		file.DriveModifiedTime.Time = fileInfo.ModifiedTime
	}

	return file
}

// WalkerStats contains walker statistics.
type WalkerStats struct {
	FoldersScanned int64
	FilesFound     int64
	TotalSize      int64
	ErrorCount     int
}
