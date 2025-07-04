package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/VatsalSy/CloudPull/internal/app"
	"github.com/VatsalSy/CloudPull/internal/state"
	"github.com/VatsalSy/CloudPull/internal/util"
	"github.com/VatsalSy/CloudPull/pkg/progress"
)

var statusCmd = &cobra.Command{
	Use:   "status [session-id]",
	Short: "Show sync progress and statistics",
	Long: `Display detailed status information about ongoing or completed sync sessions.

Shows real-time progress, transfer speeds, and estimated completion time
for active syncs.`,
	Example: `  # Show status of all active sessions
  cloudpull status

  # Show status of specific session
  cloudpull status abc123

  # Show detailed statistics
  cloudpull status --detailed

  # Monitor status continuously
  cloudpull status --watch`,
	RunE: runStatus,
}

var (
	watchStatus    bool
	detailedStatus bool
	showHistory    bool
)

func init() {
	statusCmd.Flags().BoolVarP(&watchStatus, "watch", "w", false,
		"Continuously monitor status")
	statusCmd.Flags().BoolVarP(&detailedStatus, "detailed", "d", false,
		"Show detailed statistics")
	statusCmd.Flags().BoolVar(&showHistory, "history", false,
		"Show completed sessions")
}

func runStatus(cmd *cobra.Command, args []string) error {
	if watchStatus {
		return watchSyncStatus(args)
	}

	if showHistory {
		return showSyncHistory()
	}

	return showSyncStatus(args)
}

func showSyncStatus(args []string) error {
	fmt.Println(color.CyanString("📊 CloudPull Status"))
	fmt.Println()

	// Get active sessions
	sessions := getActiveSessions()
	if len(sessions) == 0 {
		fmt.Println(color.YellowString("No active sync sessions."))
		fmt.Println("\nUse 'cloudpull sync' to start a new sync")
		fmt.Println("Use 'cloudpull status --history' to see completed sessions")
		return nil
	}

	// Show specific session or all
	if len(args) > 0 {
		sessionID := args[0]
		for _, session := range sessions {
			if session.ID == sessionID {
				return showDetailedSession(session)
			}
		}
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Show all active sessions
	showActiveSessions(sessions)

	if detailedStatus {
		fmt.Println()
		showSystemStats()
	}

	return nil
}

func showActiveSessions(sessions []ActiveSession) {
	fmt.Printf("Active Sessions: %d\n\n", len(sessions))

	for _, session := range sessions {
		// Session header
		fmt.Printf("%s Session: %s\n",
			color.GreenString("▶"),
			color.CyanString(session.ID))
		fmt.Printf("  Source: %s → %s\n", session.Source, session.Destination)

		// Progress bar
		bar := progressbar.NewOptions64(
			session.TotalBytes,
			progressbar.OptionSetDescription("  Progress"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionSetPredictTime(false),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
		bar.Set64(session.DownloadedBytes)
		fmt.Print("\n")

		// Statistics
		fmt.Printf("  Files: %d/%d (%.0f%%) | Speed: %s/s | ETA: %s\n",
			session.CompletedFiles, session.TotalFiles,
			float64(session.CompletedFiles)/float64(session.TotalFiles)*100,
			util.FormatBytes(session.Speed),
			formatDuration(session.ETA))

		if session.CurrentFile != "" {
			fmt.Printf("  Current: %s\n", color.YellowString(session.CurrentFile))
		}
		fmt.Println()
	}
}

func showDetailedSession(session ActiveSession) error {
	fmt.Printf("%s Session Details: %s\n",
		color.GreenString("▶"),
		color.CyanString(session.ID))
	fmt.Println(strings.Repeat("─", 50))

	// Basic info
	info := [][]string{
		{"Started", session.StartTime.Format("Jan 2, 2006 3:04:05 PM")},
		{"Duration", formatDuration(time.Since(session.StartTime))},
		{"Source", session.Source},
		{"Destination", session.Destination},
	}

	for _, row := range info {
		fmt.Printf("%-15s: %s\n", row[0], row[1])
	}

	fmt.Println()

	// Progress details
	fmt.Println(color.YellowString("Progress:"))
	fmt.Printf("  Files      : %d / %d (%.1f%%)\n",
		session.CompletedFiles, session.TotalFiles,
		float64(session.CompletedFiles)/float64(session.TotalFiles)*100)
	fmt.Printf("  Downloaded : %s / %s (%.1f%%)\n",
		util.FormatBytes(session.DownloadedBytes), util.FormatBytes(session.TotalBytes),
		float64(session.DownloadedBytes)/float64(session.TotalBytes)*100)
	fmt.Printf("  Remaining  : %s\n", util.FormatBytes(session.TotalBytes-session.DownloadedBytes))

	fmt.Println()

	// Transfer stats
	fmt.Println(color.YellowString("Transfer Statistics:"))
	fmt.Printf("  Current Speed : %s/s\n", util.FormatBytes(session.Speed))
	fmt.Printf("  Average Speed : %s/s\n", util.FormatBytes(session.AvgSpeed))
	fmt.Printf("  Peak Speed    : %s/s\n", util.FormatBytes(session.PeakSpeed))
	fmt.Printf("  ETA           : %s\n", formatDuration(session.ETA))

	if session.CurrentFile != "" {
		fmt.Println()
		fmt.Println(color.YellowString("Current Activity:"))
		fmt.Printf("  Downloading: %s\n", session.CurrentFile)
		fmt.Printf("  File Size  : %s\n", util.FormatBytes(session.CurrentFileSize))
		fmt.Printf("  Progress   : %.1f%%\n", session.CurrentFileProgress)
	}

	// Recent files
	if len(session.RecentFiles) > 0 {
		fmt.Println()
		fmt.Println(color.YellowString("Recently Completed:"))
		for _, file := range session.RecentFiles {
			fmt.Printf("  ✓ %s (%s)\n", file.Name, util.FormatBytes(file.Size))
		}
	}

	return nil
}

func watchSyncStatus(args []string) error {
	fmt.Println(color.CyanString("📊 CloudPull Status Monitor"))
	fmt.Println("Press Ctrl+C to exit")
	fmt.Println()

	for {
		// Clear screen (simple version)
		fmt.Print("\033[H\033[2J")

		showSyncStatus(args)

		time.Sleep(1 * time.Second)
	}
}

func showSyncHistory() error {
	fmt.Println(color.CyanString("📜 CloudPull Sync History"))
	fmt.Println()

	history := getSyncHistory()
	if len(history) == 0 {
		fmt.Println("No completed sync sessions.")
		return nil
	}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Session ID", "Date", "Duration", "Files", "Size", "Status"})

	for _, session := range history {
		status := color.GreenString("✓ Completed")
		if session.Failed {
			status = color.RedString("✗ Failed")
		} else if session.Canceled {
			status = color.YellowString("⚠ Canceled")
		}

		t.AppendRow(table.Row{
			session.ID,
			session.EndTime.Format("Jan 2 15:04"),
			formatDuration(session.Duration),
			fmt.Sprintf("%d", session.TotalFiles),
			util.FormatBytes(session.TotalBytes),
			status,
		})
	}

	t.Render()
	fmt.Printf("\nTotal: %d sessions\n", len(history))

	return nil
}

func showSystemStats() {
	fmt.Println(color.YellowString("System Statistics:"))

	stats := getSystemStats()
	fmt.Printf("  Network Usage    : %s/s ↓ / %s/s ↑\n",
		util.FormatBytes(stats.DownloadRate), util.FormatBytes(stats.UploadRate))
	fmt.Printf("  Disk Space       : %s free of %s\n",
		util.FormatBytes(stats.DiskFree), util.FormatBytes(stats.DiskTotal))
	fmt.Printf("  Memory Usage     : %.1f%% (%s / %s)\n",
		float64(stats.MemUsed)/float64(stats.MemTotal)*100,
		util.FormatBytes(stats.MemUsed), util.FormatBytes(stats.MemTotal))
	fmt.Printf("  Active Threads   : %d\n", stats.ActiveThreads)
}

type ActiveSession struct {
	StartTime           time.Time
	CurrentFile         string
	Source              string
	Destination         string
	ID                  string
	RecentFiles         []CompletedFile
	TotalFiles          int
	DownloadedBytes     int64
	Speed               int64
	AvgSpeed            int64
	PeakSpeed           int64
	ETA                 time.Duration
	TotalBytes          int64
	CurrentFileSize     int64
	CurrentFileProgress float64
	CompletedFiles      int
}

type CompletedFile struct {
	Name string
	Size int64
}

type SystemStats struct {
	DownloadRate  int64
	UploadRate    int64
	DiskFree      int64
	DiskTotal     int64
	MemUsed       int64
	MemTotal      int64
	ActiveThreads int
}

func getActiveSessions() []ActiveSession {
	app, err := getOrCreateApp()
	if err != nil {
		return []ActiveSession{}
	}

	ctx := context.Background()
	sessions, err := app.GetSessions(ctx)
	if err != nil {
		return []ActiveSession{}
	}

	var activeSessions []ActiveSession
	for _, session := range sessions {
		if session.Status == "active" || session.Status == "paused" {
			activeSessions = append(activeSessions, convertToActiveSession(session))
		}
	}

	return activeSessions
}

func getSyncHistory() []SyncSession {
	app, err := getOrCreateApp()
	if err != nil {
		return []SyncSession{}
	}

	ctx := context.Background()
	sessions, err := app.GetSessions(ctx)
	if err != nil {
		return []SyncSession{}
	}

	var history []SyncSession
	for _, session := range sessions {
		if session.Status == "completed" || session.Status == "failed" || session.Status == "canceled" {
			history = append(history, convertToSyncSession(session))
		}
	}

	return history
}

func getSystemStats() SystemStats {
	// Get aggregate stats from all active sessions
	var totalDownloadRate, totalUploadRate int64
	var activeThreads int

	sessions := getActiveSessions()
	for _, session := range sessions {
		totalDownloadRate += session.Speed
		activeThreads++ // Each session represents at least one thread
	}

	// Get disk stats
	diskFree, diskTotal := getDiskStats()

	// Get memory stats
	memUsed, memTotal := getMemoryStats()

	return SystemStats{
		DownloadRate:  totalDownloadRate,
		UploadRate:    totalUploadRate,
		DiskFree:      diskFree,
		DiskTotal:     diskTotal,
		MemUsed:       memUsed,
		MemTotal:      memTotal,
		ActiveThreads: activeThreads,
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// Progress tracking integration.
var (
	progressTrackers  = make(map[string]*progress.Tracker)
	metricsCollectors = make(map[string]*progress.MetricsCollector)
	trackerMu         sync.RWMutex
)

// getProgressTracker returns the progress tracker for a session.
func getProgressTracker(sessionID string) *progress.Tracker {
	trackerMu.RLock()
	defer trackerMu.RUnlock()
	return progressTrackers[sessionID]
}

// getMetricsCollector returns the metrics collector for a session.
func getMetricsCollector(sessionID string) *progress.MetricsCollector {
	trackerMu.RLock()
	defer trackerMu.RUnlock()
	return metricsCollectors[sessionID]
}

// RegisterProgressTracker registers a progress tracker for a session.
func RegisterProgressTracker(sessionID string, tracker *progress.Tracker,
	metrics *progress.MetricsCollector) {

	trackerMu.Lock()
	defer trackerMu.Unlock()
	progressTrackers[sessionID] = tracker
	metricsCollectors[sessionID] = metrics
}

// UnregisterProgressTracker removes a progress tracker for a session.
func UnregisterProgressTracker(sessionID string) {
	trackerMu.Lock()
	defer trackerMu.Unlock()
	delete(progressTrackers, sessionID)
	delete(metricsCollectors, sessionID)
}

// getRecentFiles returns recently completed files for a session.
func getRecentFiles(sessionID string, limit int) []CompletedFile {
	// TODO: This should be passed from the app context
	// For now, return empty since we don't have a global state manager
	return []CompletedFile{}
}

// getDiskStats returns disk usage statistics.
func getDiskStats() (free, total int64) {
	// TODO: Implement actual disk stats using syscall
	// For now, return placeholder values
	return 1024 * 1024 * 1024 * 120, 1024 * 1024 * 1024 * 500
}

// getMemoryStats returns memory usage statistics.
func getMemoryStats() (used, total int64) {
	// TODO: Implement actual memory stats using runtime
	// For now, return placeholder values
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// #nosec G115 - memory values are always positive and within int64 range
	return int64(m.Alloc), int64(m.Sys)
}

// SyncSession represents a completed sync session.
type SyncSession struct {
	StartTime  time.Time
	EndTime    time.Time
	ID         string
	Duration   time.Duration
	TotalFiles int
	TotalBytes int64
	Failed     bool
	Canceled   bool
}

// safeUint64ToInt safely converts uint64 to int, capping at MaxInt.
func safeUint64ToInt(n uint64) int {
	if n > math.MaxInt {
		return math.MaxInt
	}
	return int(n)
}

func safeInt64ToInt(n int64) int {
	if n > math.MaxInt {
		return math.MaxInt
	}
	if n < math.MinInt {
		return math.MinInt
	}
	return int(n)
}

// getOrCreateApp returns a shared app instance.
func getOrCreateApp() (*app.App, error) {
	application, err := app.New()
	if err != nil {
		return nil, err
	}

	if err := application.Initialize(); err != nil {
		return nil, err
	}

	return application, nil
}

// convertToActiveSession converts a state.Session to ActiveSession.
func convertToActiveSession(session *state.Session) ActiveSession {
	var eta time.Duration
	if session.CompletedBytes > 0 && session.TotalBytes > 0 {
		elapsed := time.Since(session.StartTime)
		progress := float64(session.CompletedBytes) / float64(session.TotalBytes)
		if progress > 0 {
			totalTime := time.Duration(float64(elapsed) / progress)
			eta = totalTime - elapsed
		}
	}

	var speed int64
	if elapsed := time.Since(session.StartTime); elapsed > 0 {
		speed = int64(float64(session.CompletedBytes) / elapsed.Seconds())
	}

	source := "Unknown"
	if session.RootFolderName.Valid {
		source = session.RootFolderName.String
	}

	return ActiveSession{
		ID:              session.ID,
		StartTime:       session.StartTime,
		Source:          source,
		Destination:     session.DestinationPath,
		TotalFiles:      safeInt64ToInt(session.TotalFiles),
		CompletedFiles:  safeInt64ToInt(session.CompletedFiles),
		TotalBytes:      session.TotalBytes,
		DownloadedBytes: session.CompletedBytes,
		Speed:           speed,
		AvgSpeed:        speed,
		PeakSpeed:       speed,
		ETA:             eta,
	}
}

// convertToSyncSession converts a state.Session to SyncSession.
func convertToSyncSession(session *state.Session) SyncSession {
	endTime := time.Now()
	if session.EndTime.Valid {
		endTime = session.EndTime.Time
	}

	return SyncSession{
		ID:         session.ID,
		StartTime:  session.StartTime,
		EndTime:    endTime,
		Duration:   endTime.Sub(session.StartTime),
		TotalFiles: safeInt64ToInt(session.TotalFiles),
		TotalBytes: session.TotalBytes,
		Failed:     session.Status == state.SessionStatusFailed,
		Canceled:   session.Status == state.SessionStatusCancelled,
	}
}
