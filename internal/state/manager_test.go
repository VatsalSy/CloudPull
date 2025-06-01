package state_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
	"errors" // Added for TestManager_LogError

	"github.com/VatsalSy/CloudPull/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*state.Manager, func()) {
	t.Helper()
	dbPath := ":memory:"
	cfg := state.DBConfig{
		Path:         dbPath,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
		MaxIdleTime:  5 * time.Minute,
	}
	if dbPath != ":memory:" {
		err := os.MkdirAll(filepath.Dir(dbPath), 0750)
		require.NoError(t, err, "Failed to create directory for test DB")
	}
	manager, err := state.NewManager(cfg)
	require.NoError(t, err, "state.NewManager failed")
	require.NotNil(t, manager, "state.Manager is nil")
	cleanup := func() {
		err := manager.Close()
		assert.NoError(t, err, "manager.Close() failed during cleanup")
	}
	return manager, cleanup
}

func TestManager_CreateSession(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	rootFolderID := "testRootFolderID123"
	rootFolderName := "My Test Root Folder"
	destinationPath := "/tmp/cloudpull_test_dest"

	session, err := manager.CreateSession(ctx, rootFolderID, rootFolderName, destinationPath)
	require.NoError(t, err, "CreateSession failed")
	require.NotNil(t, session, "Created session is nil")

	assert.NotEmpty(t, session.ID, "Session ID should not be empty")
	assert.Equal(t, rootFolderID, session.RootFolderID, "RootFolderID does not match")
	require.True(t, session.RootFolderName.Valid, "RootFolderName should be valid")
	assert.Equal(t, rootFolderName, session.RootFolderName.String, "RootFolderName does not match")
	assert.Equal(t, destinationPath, session.DestinationPath, "DestinationPath does not match")
	assert.Equal(t, state.SessionStatusActive, session.Status, "Session status should be Active")
	assert.False(t, session.StartTime.IsZero(), "StartTime should be set")
	assert.WithinDuration(t, time.Now(), session.StartTime, 5*time.Second, "StartTime is not recent")
	assert.False(t, session.EndTime.Valid, "EndTime should not be valid for an active session")
}

func TestManager_GetSession(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	rootFolderID := "testGetRootID"
	rootFolderName := "GetSession Test Folder"
	destinationPath := "/tmp/get_session"

	createdSession, err := manager.CreateSession(ctx, rootFolderID, rootFolderName, destinationPath)
	require.NoError(t, err)
	require.NotNil(t, createdSession)

	retrievedSession, err := manager.GetSession(ctx, createdSession.ID)
	require.NoError(t, err, "GetSession failed for existing ID")
	require.NotNil(t, retrievedSession, "Retrieved session is nil for existing ID")

	assert.Equal(t, createdSession.ID, retrievedSession.ID)
	assert.Equal(t, createdSession.RootFolderID, retrievedSession.RootFolderID)
	assert.Equal(t, createdSession.RootFolderName, retrievedSession.RootFolderName) // Direct compare ok for sql.NullString
	assert.Equal(t, createdSession.DestinationPath, retrievedSession.DestinationPath)
	assert.Equal(t, createdSession.Status, retrievedSession.Status)
	assert.WithinDuration(t, createdSession.StartTime, retrievedSession.StartTime, time.Second)
	assert.Equal(t, createdSession.EndTime.Valid, retrievedSession.EndTime.Valid)
	if createdSession.EndTime.Valid {
		assert.WithinDuration(t, createdSession.EndTime.Time, retrievedSession.EndTime.Time, time.Second)
	}

	nonExistentID := "nonExistentSessionID12345"
	notFoundSession, err := manager.GetSession(ctx, nonExistentID)
	assert.Error(t, err, "Expected an error when getting a non-existent session")
	assert.Nil(t, notFoundSession, "Session should be nil for non-existent ID")
}

func TestManager_UpdateSessionStatus(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	createdSession, err := manager.CreateSession(ctx, "updateStatusID", "UpdateStatusFolder", "/tmp/update_status")
	require.NoError(t, err)
	require.NotNil(t, createdSession)
	assert.Equal(t, state.SessionStatusActive, createdSession.Status, "Initial status should be Active")
	assert.False(t, createdSession.EndTime.Valid, "EndTime should not be valid for active session")

	err = manager.UpdateSessionStatus(ctx, createdSession.ID, state.SessionStatusPaused)
	require.NoError(t, err, "UpdateSessionStatus to Paused failed")
	updatedSession, err := manager.GetSession(ctx, createdSession.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedSession)
	assert.Equal(t, state.SessionStatusPaused, updatedSession.Status, "Session status not updated to Paused")
	assert.False(t, updatedSession.EndTime.Valid, "EndTime should NOT be valid when session is Paused via generic UpdateSessionStatus")

	err = manager.UpdateSessionStatus(ctx, createdSession.ID, state.SessionStatusCompleted)
	require.NoError(t, err, "UpdateSessionStatus to Completed failed")
	completedSession, err := manager.GetSession(ctx, createdSession.ID)
	require.NoError(t, err)
	require.NotNil(t, completedSession)
	assert.Equal(t, state.SessionStatusCompleted, completedSession.Status, "Session status not updated to Completed")
	assert.False(t, completedSession.EndTime.Valid, "EndTime should NOT be valid when session is Completed via generic UpdateSessionStatus")
}

func TestManager_CreateFolder_GetFolder(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	session, err := manager.CreateSession(ctx, "s1", "Session1", "/dest1")
	require.NoError(t, err)

	rootFolderData := state.Folder{DriveID: "driveRootFolder1", SessionID: session.ID, Name: "Root Test Folder", Path: "/Root Test Folder", Status: state.FolderStatusPending}
	err = manager.CreateFolder(ctx, &rootFolderData)
	require.NoError(t, err, "CreateFolder for root failed")
	require.NotEmpty(t, rootFolderData.ID, "Root folder ID should be populated")

	retrievedRootFolder, err := manager.Folders().Get(ctx, rootFolderData.ID)
	require.NoError(t, err, "Failed to get root folder")
	require.NotNil(t, retrievedRootFolder)
	assert.Equal(t, rootFolderData.Name, retrievedRootFolder.Name)

	childFolderData := state.Folder{DriveID: "driveChildFolder1", SessionID: session.ID, Name: "Child Test Folder", Path: "/Root Test Folder/Child Test Folder", Status: state.FolderStatusPending, ParentID:  sql.NullString{String: rootFolderData.ID, Valid: true}}
	err = manager.CreateFolder(ctx, &childFolderData)
	require.NoError(t, err, "CreateFolder for child failed")
	require.NotEmpty(t, childFolderData.ID)

	retrievedChildFolder, err := manager.Folders().Get(ctx, childFolderData.ID)
	require.NoError(t, err, "Failed to get child folder")
	require.NotNil(t, retrievedChildFolder)
	assert.Equal(t, childFolderData.Name, retrievedChildFolder.Name)
	assert.Equal(t, rootFolderData.ID, retrievedChildFolder.ParentID.String)
}

func TestManager_CreateFiles_GetFile(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	session, err := manager.CreateSession(ctx, "s2", "SessionFiles", "/dest_files")
	require.NoError(t, err)
	folderData := state.Folder{DriveID: "folderForFiles1", SessionID: session.ID, Name: "Files Test Folder", Path: "/Files Test Folder", Status: state.FolderStatusPending}
	err = manager.CreateFolder(ctx, &folderData)
	require.NoError(t, err)

	filesToCreate := []*state.File{
		{DriveID: "driveFile1", FolderID: folderData.ID, SessionID: session.ID, Name: "file1.txt", Path: "/Files Test Folder/file1.txt", Size: 1024, Status: state.FileStatusPending, IsGoogleDoc: false},
		{DriveID: "driveFile2", FolderID: folderData.ID, SessionID: session.ID, Name: "gdoc1", Path: "/Files Test Folder/gdoc1", Size: 0, Status: state.FileStatusPending, IsGoogleDoc: true, ExportMimeType: sql.NullString{String: "application/pdf", Valid: true}},
	}
	err = manager.CreateFiles(ctx, filesToCreate)
	require.NoError(t, err, "CreateFiles failed")

	fileToVerify := filesToCreate[0]
	retrievedFile, err := manager.Files().Get(ctx, fileToVerify.ID)
	require.NoError(t, err, "Failed to get file")
	require.NotNil(t, retrievedFile)
	assert.Equal(t, fileToVerify.Name, retrievedFile.Name)
}

func TestManager_GetNextPendingFolder(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	session, err := manager.CreateSession(ctx, "s3", "PendingFolderSession", "/dest_pending_folder")
	require.NoError(t, err)

	folderDataZ := state.Folder{DriveID: "pendingF_Z", SessionID: session.ID, Name: "Folder Z", Path: "/Folder Z", Status: state.FolderStatusPending}
	err = manager.CreateFolder(ctx, &folderDataZ)
	require.NoError(t, err)
	folderDataA := state.Folder{DriveID: "pendingF_A", SessionID: session.ID, Name: "Folder A", Path: "/Folder A", Status: state.FolderStatusPending}
	err = manager.CreateFolder(ctx, &folderDataA)
	require.NoError(t, err)

	nextFolder, err := manager.GetNextPendingFolder(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, nextFolder)
	assert.Equal(t, folderDataA.ID, nextFolder.ID)

	nextFolder.Status = state.FolderStatusScanned
	err = manager.Folders().Update(ctx, nextFolder)
	require.NoError(t, err)

	nextFolder, err = manager.GetNextPendingFolder(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, nextFolder)
	assert.Equal(t, folderDataZ.ID, nextFolder.ID)

	nextFolder.Status = state.FolderStatusScanned
	err = manager.Folders().Update(ctx, nextFolder)
	require.NoError(t, err)

	nextFolder, err = manager.GetNextPendingFolder(ctx, session.ID)
	require.NoError(t, err)
	assert.Nil(t, nextFolder)
}

func TestManager_GetNextPendingFile(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	session, err := manager.CreateSession(ctx, "s4", "PendingFileSession", "/dest_pending_file")
	require.NoError(t, err)
	folder := state.Folder{DriveID: "folderForPendingFiles", SessionID: session.ID, Name: "PF Folder", Path: "/PF Folder", Status: state.FolderStatusScanned}
	err = manager.CreateFolder(ctx, &folder)
	require.NoError(t, err)

	filePartial := state.File{DriveID: "filePartial", FolderID: folder.ID, SessionID: session.ID, Name: "partial.txt", Path: "/PF Folder/partial.txt", Size: 2048, Status: state.FileStatusDownloading, BytesDownloaded: 1024}
	fileSmallPending := state.File{DriveID: "fileSmall", FolderID: folder.ID, SessionID: session.ID, Name: "small.txt", Path: "/PF Folder/small.txt", Size: 512, Status: state.FileStatusPending}
	fileLargePending := state.File{DriveID: "fileLarge", FolderID: folder.ID, SessionID: session.ID, Name: "large.txt", Path: "/PF Folder/large.txt", Size: 4096, Status: state.FileStatusPending}
	err = manager.CreateFiles(ctx, []*state.File{&filePartial, &fileSmallPending, &fileLargePending})
	require.NoError(t, err)

	nextFile, err := manager.GetNextPendingFile(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, nextFile)
	assert.Equal(t, filePartial.ID, nextFile.ID)

	err = manager.MarkFileComplete(ctx, filePartial.ID, session.ID)
	require.NoError(t, err)

	nextFile, err = manager.GetNextPendingFile(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, nextFile)
	assert.Equal(t, fileSmallPending.ID, nextFile.ID)

	err = manager.MarkFileComplete(ctx, fileSmallPending.ID, session.ID)
	require.NoError(t, err)

	nextFile, err = manager.GetNextPendingFile(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, nextFile)
	assert.Equal(t, fileLargePending.ID, nextFile.ID)

	err = manager.MarkFileComplete(ctx, fileLargePending.ID, session.ID)
	require.NoError(t, err)

	nextFile, err = manager.GetNextPendingFile(ctx, session.ID)
	require.NoError(t, err)
	assert.Nil(t, nextFile)
}

func TestManager_MarkFileComplete(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	var err error // Declare err for the function scope

	session, err := manager.CreateSession(ctx, "s5", "MarkCompleteSession", "/dest_mc")
	require.NoError(t, err)
	folderData := state.Folder{DriveID: "f_mc", SessionID: session.ID, Name: "MC Folder", Path: "/MC Folder", Status: state.FolderStatusScanned}
	err = manager.CreateFolder(ctx, &folderData)
	require.NoError(t, err)
	fileToComplete := state.File{DriveID: "file_mc", FolderID: folderData.ID, SessionID: session.ID, Name: "complete_me.txt", Path: "/MC Folder/complete_me.txt", Size: 1024, Status: state.FileStatusPending}
	err = manager.CreateFiles(ctx, []*state.File{&fileToComplete})
	require.NoError(t, err)

	initialProgress, err := manager.Queries().GetSessionProgress(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, initialProgress)

	err = manager.MarkFileComplete(ctx, fileToComplete.ID, session.ID)
	require.NoError(t, err, "MarkFileComplete failed")

	// Use a new variable name to ensure it's new in this specific assignment
	completedFile, err := manager.Files().Get(ctx, fileToComplete.ID)
	require.NoError(t, err)
	require.NotNil(t, completedFile)
	assert.Equal(t, state.FileStatusCompleted, completedFile.Status, "File status not updated to Completed")

	finalProgress, err := manager.Queries().GetSessionProgress(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, finalProgress)
	assert.Equal(t, initialProgress.CompletedFiles+1, finalProgress.CompletedFiles, "CompletedFiles count did not increment")
	assert.Equal(t, initialProgress.CompletedBytes+fileToComplete.Size, finalProgress.CompletedBytes, "CompletedBytes did not increment correctly")
}

func TestManager_MarkFileFailed(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	var err error // Declare err for the function scope

	session, err := manager.CreateSession(ctx, "s6", "MarkFailedSession", "/dest_mf")
	require.NoError(t, err)
	folderData := state.Folder{DriveID: "f_mf", SessionID: session.ID, Name: "MF Folder", Path: "/MF Folder", Status: state.FolderStatusScanned}
	err = manager.CreateFolder(ctx, &folderData)
	require.NoError(t, err)
	fileToFail := state.File{DriveID: "file_mf", FolderID: folderData.ID, SessionID: session.ID, Name: "fail_me.txt", Path: "/MF Folder/fail_me.txt", Size: 500, Status: state.FileStatusPending}
	err = manager.CreateFiles(ctx, []*state.File{&fileToFail})
	require.NoError(t, err)

	initialProgress, err := manager.Queries().GetSessionProgress(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, initialProgress)

	testErr := os.ErrNotExist
	err = manager.MarkFileFailed(ctx, fileToFail.ID, session.ID, testErr)
	require.NoError(t, err, "MarkFileFailed failed")

	// Use a new variable name here too
	failedFile, err := manager.Files().Get(ctx, fileToFail.ID)
	require.NoError(t, err)
	require.NotNil(t, failedFile)
	assert.Equal(t, state.FileStatusFailed, failedFile.Status, "File status not updated to Failed")
	require.True(t, failedFile.ErrorMessage.Valid, "ErrorMessage should be valid")
	assert.Equal(t, testErr.Error(), failedFile.ErrorMessage.String, "ErrorMessage not set correctly")

	finalProgress, err := manager.Queries().GetSessionProgress(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, finalProgress)
	assert.Equal(t, initialProgress.FailedFiles+1, finalProgress.FailedFiles, "FailedFiles count did not increment")

	var errorLog []state.ErrorLog
	err = manager.DB().SelectContext(ctx, &errorLog, "SELECT * FROM error_log WHERE session_id = $1 AND item_id = $2", session.ID, fileToFail.ID)
	require.NoError(t, err, "Failed to query error_log")
	require.Len(t, errorLog, 1, "Expected one entry in error_log")
	assert.Equal(t, fileToFail.ID, errorLog[0].ItemID)
	assert.Equal(t, "file", errorLog[0].ItemType)
	assert.Equal(t, "download_failed", errorLog[0].ErrorType)
	require.True(t, errorLog[0].ErrorMessage.Valid)
	assert.Equal(t, testErr.Error(), errorLog[0].ErrorMessage.String)
}

func TestManager_LogError(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	session, err := manager.CreateSession(ctx, "s_logerr", "LogErrorSession", "/logerr")
	require.NoError(t, err)

	testFileID := "file_logerr_1"
	testErrorType := "test_download_issue"
	originalError := errors.New("a unique test error occurred for LogError")

	err = manager.LogError(ctx, session.ID, testFileID, "file", testErrorType, originalError)
	require.NoError(t, err, "manager.LogError itself failed")

	var loggedErrors []state.ErrorLog
	// Using $1, $2 for SQLite placeholders, as sqlx often uses ? internally but then rebinds.
	// Standard Go database/sql uses $n for Postgres, ? for MySQL/SQLite. sqlx should handle this.
	query := "SELECT * FROM error_log WHERE session_id = $1 AND item_id = $2 AND error_type = $3"
	err = manager.DB().SelectContext(ctx, &loggedErrors, query, session.ID, testFileID, testErrorType)
	require.NoError(t, err, "Failed to query error_log table")
	require.Len(t, loggedErrors, 1, "Expected exactly one error log entry")

	loggedError := loggedErrors[0]
	assert.Equal(t, session.ID, loggedError.SessionID)
	assert.Equal(t, testFileID, loggedError.ItemID)
	assert.Equal(t, "file", loggedError.ItemType)
	assert.Equal(t, testErrorType, loggedError.ErrorType)
	require.True(t, loggedError.ErrorMessage.Valid, "Error message should be valid in log")
	assert.Equal(t, originalError.Error(), loggedError.ErrorMessage.String)
	require.True(t, loggedError.StackTrace.Valid, "Stack trace should be valid in log")
	assert.NotEmpty(t, loggedError.StackTrace.String, "Stack trace should not be empty")
	assert.True(t, loggedError.IsRetryable, "IsRetryable should be true by default")
}

func TestManager_ResumeSession(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	var err error

	session, err := manager.CreateSession(ctx, "s_resume", "ResumeSession", "/resume")
	require.NoError(t, err)

	// Set session to a resumable status (e.g., failed)
	err = manager.UpdateSessionStatus(ctx, session.ID, state.SessionStatusFailed)
	require.NoError(t, err)

	folder := state.Folder{DriveID: "folder_resume", SessionID: session.ID, Name: "ResumeF", Path: "/ResumeF", Status: state.FolderStatusFailed}
	err = manager.CreateFolder(ctx, &folder)
	require.NoError(t, err)

	file := state.File{DriveID: "file_resume", FolderID: folder.ID, SessionID: session.ID, Name: "resume.txt", Path: "/ResumeF/resume.txt", Size: 100, Status: state.FileStatusFailed, DownloadAttempts: 1}
	err = manager.CreateFiles(ctx, []*state.File{&file})
	require.NoError(t, err)

	err = manager.ResumeSession(ctx, session.ID)
	require.NoError(t, err, "ResumeSession failed")

	resumedSession, err := manager.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, resumedSession)
	assert.Equal(t, state.SessionStatusActive, resumedSession.Status, "Session status not updated to Active after resume")

	resumedFile, err := manager.Files().Get(ctx, file.ID)
	require.NoError(t, err)
	require.NotNil(t, resumedFile)
	// ResetFailedFiles (called by ResumeSession) should set status to Pending and reset attempts.
	assert.Equal(t, state.FileStatusPending, resumedFile.Status, "File status not updated to Pending after resume")
	assert.Equal(t, 0, resumedFile.DownloadAttempts, "File download attempts should be reset")


	resumedFolder, err := manager.Folders().Get(ctx, folder.ID)
	require.NoError(t, err)
	require.NotNil(t, resumedFolder)
	assert.Equal(t, state.FolderStatusPending, resumedFolder.Status, "Folder status not updated to Pending after resume")
}

func TestManager_GetSessionStats(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	var err error

	session, err := manager.CreateSession(ctx, "s_stats", "StatsSession", "/stats")
	require.NoError(t, err)

	folder := state.Folder{DriveID: "f_stats", SessionID: session.ID, Name: "StatsF", Path: "/StatsF", Status: state.FolderStatusPending}
	err = manager.CreateFolder(ctx, &folder) // Will be pending
	require.NoError(t, err)

	fileCompleted := state.File{DriveID: "f_stats_comp", FolderID: folder.ID, SessionID: session.ID, Name: "completed.txt", Path: "/StatsF/completed.txt", Size: 100, Status: state.FileStatusPending}
	err = manager.CreateFiles(ctx, []*state.File{&fileCompleted})
	require.NoError(t, err)
	// Manually update session totals for this file before marking complete, as CreateFiles doesn't update session totals.
	err = manager.UpdateSessionTotals(ctx, session.ID, 1, 100)
	require.NoError(t, err)
	err = manager.MarkFileComplete(ctx, fileCompleted.ID, session.ID) // This updates progress counts
	require.NoError(t, err)


	fileFailed := state.File{DriveID: "f_stats_fail", FolderID: folder.ID, SessionID: session.ID, Name: "failed.txt", Path: "/StatsF/failed.txt", Size: 50, Status: state.FileStatusPending}
	err = manager.CreateFiles(ctx, []*state.File{&fileFailed})
	require.NoError(t, err)
	// Update session totals for this second file
	err = manager.UpdateSessionTotals(ctx, session.ID, 1, 50) // Adds 1 to total_files, 50 to total_bytes
	require.NoError(t, err)
	testErr := errors.New("stat fail error")
	err = manager.MarkFileFailed(ctx, fileFailed.ID, session.ID, testErr) // This updates progress counts and logs error
	require.NoError(t, err)


	stats, err := manager.GetSessionStats(ctx, session.ID)
	require.NoError(t, err, "GetSessionStats failed")
	require.NotNil(t, stats, "SessionStats is nil")

	// Check Progress part of SessionStats (which comes from session_summary view, based on session table)
	require.NotNil(t, stats.Progress, "stats.Progress is nil")
	assert.Equal(t, int64(1), stats.Progress.CompletedFiles, "Progress.CompletedFiles mismatch")
	assert.Equal(t, int64(1), stats.Progress.FailedFiles, "Progress.FailedFiles mismatch")
	assert.Equal(t, int64(100), stats.Progress.CompletedBytes, "Progress.CompletedBytes mismatch")
	assert.Equal(t, int64(2), stats.Progress.TotalFiles, "Progress.TotalFiles mismatch")
	assert.Equal(t, int64(150), stats.Progress.TotalBytes, "Progress.TotalBytes mismatch")


	require.NotNil(t, stats.Files, "stats.Files is nil")
	assert.Equal(t, int64(1), stats.Files.CompletedCount, "Files.CompletedCount mismatch")
	assert.Equal(t, int64(1), stats.Files.FailedCount, "Files.FailedCount mismatch")

	require.NotNil(t, stats.FolderCounts, "stats.FolderCounts is nil")
	assert.Equal(t, int64(1), stats.FolderCounts[state.FolderStatusPending], "Pending folder count mismatch")

	require.NotNil(t, stats.Errors, "stats.Errors is nil")
	assert.Len(t, stats.Errors, 1, "Expected one error summary")
	if len(stats.Errors) == 1 {
		assert.Equal(t, testErr.Error(), stats.Errors[0].MostRecentErrorMessage, "Error message in summary mismatch")
		assert.Equal(t, int64(1), stats.Errors[0].ErrorCount, "Error count in summary mismatch")
	}
}

func TestManager_HealthCheck(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	err := manager.HealthCheck(ctx)
	assert.NoError(t, err, "HealthCheck failed")
}

func TestManager_Vacuum(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	err := manager.Vacuum(ctx)
	// Vacuum on an in-memory DB might be a no-op or not strictly necessary,
	// but the command should execute without error.
	assert.NoError(t, err, "Vacuum failed")
}

func TestManager_GetSetConfig_DB(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	var err error

	key := "db_config_key_test_123"
	value := "db_config_value_xyz"

	err = manager.SetConfig(ctx, key, value)
	require.NoError(t, err, "SetConfig failed")

	retrievedValue, err := manager.GetConfig(ctx, key)
	require.NoError(t, err, "GetConfig failed for existing key")
	assert.Equal(t, value, retrievedValue, "Retrieved value does not match set value")

	_, err = manager.GetConfig(ctx, "this_key_does_not_exist_XYZ")
	assert.Error(t, err, "Expected error when getting a non-existent key")
	// TODO: Check for a specific error type if defined, e.g., errors.Is(err, state.ErrConfigNotFound)
	// For now, a generic error check is fine.
}
