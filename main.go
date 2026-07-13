package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.design/x/clipboard"
)

const (
	DefaultCleanupTTL = 1 * time.Hour
	FilenamePrefix    = "mcp-clip-"
	CurrentResourceURI = "mcp-clip://current"
)

type clipboardState struct {
	content string
	time    time.Time
}

type ClipboardServer struct {
	lastClipboard atomic.Value                       // stores clipboardState
	running       int32                              // atomic flag for monitoring state
	cancel        atomic.Pointer[context.CancelFunc] // FIXED: Now uses atomic pointer
	sessionFiles  []string                           // track files created during this session
	filesMutex    sync.Mutex                         // protect sessionFiles slice
	mcpServer     *server.MCPServer                  // reference to MCP server
	resourceRegistered bool                          // track if current resource is registered
}


func NewClipboardServer() *ClipboardServer {
	cs := &ClipboardServer{}
	cs.lastClipboard.Store(clipboardState{})
	return cs
}

// updateClipboard updates clipboard state with simple atomic store.
// Returns true if content changed, false if content was already present.
func (cs *ClipboardServer) updateClipboard(content string) bool {
	current := cs.lastClipboard.Load()
	currentState, ok := current.(clipboardState)
	if !ok {
		currentState = clipboardState{}
	}

	// Only update if content has changed
	if content == currentState.content {
		return false
	}

	newState := clipboardState{
		content: content,
		time:    time.Now(),
	}

	// Simple atomic store - good enough for clipboard updates
	cs.lastClipboard.Store(newState)
	
	// Send notification about clipboard change
	cs.sendClipboardChangeNotification(content)
	
	return true
}

// sendClipboardChangeNotification sends an MCP notification when clipboard changes
func (cs *ClipboardServer) sendClipboardChangeNotification(content string) {
	if cs.mcpServer == nil {
		return
	}
	
	// Determine content type and create appropriate notification
	var notificationData map[string]interface{}
	
	if content == "" {
		notificationData = map[string]interface{}{
			"type": "empty",
			"timestamp": time.Now().Unix(),
		}
	} else if isProbablyText(content) {
		contentPreview := content
		if len(content) > 200 {
			contentPreview = content[:200] + "..."
		}
		
		notificationData = map[string]interface{}{
			"type": "text",
			"size": len(content),
			"preview": contentPreview,
			"timestamp": time.Now().Unix(),
			"resource_uri": CurrentResourceURI,
		}
	} else {
		// Binary content
		isImage, imageType := detectImageType([]byte(content))
		contentType := "binary"
		if isImage {
			contentType = "image"
		}
		
		notificationData = map[string]interface{}{
			"type": contentType,
			"format": imageType,
			"size": len(content),
			"timestamp": time.Now().Unix(),
			"resource_uri": CurrentResourceURI,
		}
	}
	
	// Note: In a real implementation, we'd need access to the MCP server's notification sending mechanism
	// This is a placeholder for the notification system
	if os.Getenv("MCP_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "Clipboard change notification: %+v\n", notificationData)
	}
}

func (cs *ClipboardServer) getLastClipboard() (string, time.Time) {
	if state, ok := cs.lastClipboard.Load().(clipboardState); ok {
		return state.content, state.time
	}
	return "", time.Time{}
}

func (cs *ClipboardServer) addSessionFile(filePath string) {
	cs.filesMutex.Lock()
	defer cs.filesMutex.Unlock()
	// Don't track files during shutdown to prevent race condition
	if atomic.LoadInt32(&cs.running) == 0 {
		return
	}
	cs.sessionFiles = append(cs.sessionFiles, filePath)
}

func (cs *ClipboardServer) cleanupSessionFiles() {
	cs.filesMutex.Lock()
	files := make([]string, len(cs.sessionFiles))
	copy(files, cs.sessionFiles)
	cs.sessionFiles = nil
	cs.filesMutex.Unlock()

	var removed, errors int
	for _, filePath := range files {
		if err := os.Remove(filePath); err != nil {
			errors++
			if os.Getenv("MCP_DEBUG") == "1" {
				fmt.Fprintf(os.Stderr, "Failed to remove session file %s: %v\n", filePath, err)
			}
		} else {
			removed++
			if os.Getenv("MCP_DEBUG") == "1" {
				fmt.Fprintf(os.Stderr, "Removed session file: %s\n", filePath)
			}
		}
	}

	// Resource is registered once at startup, no need to clear

	if os.Getenv("MCP_DEBUG") == "1" && (removed > 0 || errors > 0) {
		fmt.Fprintf(os.Stderr, "Session cleanup: %d removed, %d errors\n", removed, errors)
	}
}

func (cs *ClipboardServer) stop() {
	if atomic.CompareAndSwapInt32(&cs.running, 1, 0) {
		// Clean up session files on graceful shutdown
		cs.cleanupSessionFiles()

		// FIXED: Use atomic pointer load
		if cancelPtr := cs.cancel.Load(); cancelPtr != nil {
			(*cancelPtr)()
		}
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help":
			printUsage()
			return
		case "test":
			handleTestCommand()
			return
		case "version":
			fmt.Println("MCP Clipboard Server v1.0.0")
			return
		default:
			if strings.HasPrefix(os.Args[1], "-") {
				fmt.Printf("Unknown flag: %s\n", os.Args[1])
				printUsage()
				return
			}
		}
	}

	if isRunningFromCLI() {
		fmt.Printf("MCP Clipboard Server v1.0.0\n")
		fmt.Printf("This is an MCP (Model Context Protocol) server for clipboard access.\n")
		fmt.Printf("It should be run by an MCP client, not directly from the command line.\n\n")
		printUsage()
		return
	}

	clipboardServer := NewClipboardServer()

	// Cleanup orphaned temp files from previous instances on startup
	if os.Getenv("MCP_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "Running startup cleanup...\n")
	}
	if err := cleanupExpiredFiles(); err != nil {
		// Log warning but don't fail startup
		if os.Getenv("MCP_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "Startup cleanup warning: %v\n", err)
		}
	}

	s := server.NewMCPServer(
		"mcp-clip",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
	)

	readClipboardTool := mcp.NewTool("read_clipboard",
		mcp.WithDescription("Read the current clipboard content, supporting text and images"),
		mcp.WithString("format",
			mcp.Description("Format to return clipboard content in: 'text', 'base64', 'file', or 'auto' (default)"),
		),
		mcp.WithString("output_path",
			mcp.Description("Optional: Save binary content directly to specified file path (Windows only for images/binary)"),
		),
		mcp.WithString("chunk_size",
			mcp.Description("Optional: For large content, split into chunks of this size (bytes). Default: 25000"),
		),
		mcp.WithString("chunk_index",
			mcp.Description("Optional: Return specific chunk index (0-based) for large content"),
		),
	)

	s.AddTool(readClipboardTool, clipboardServer.readClipboardHandler)

	// Set reference to MCP server for resource management
	clipboardServer.mcpServer = s
	
	// Register the current clipboard resource
	clipboardServer.registerCurrentResource()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	clipboardServer.cancel.Store(&cancel)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		clipboardServer.stop()
		cancel()
	}()

	// Start clipboard monitoring with context
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "Clipboard monitoring panic: %v\n", r)
			}
		}()
		clipboardServer.startClipboardMonitoring(ctx)
	}()

	if err := server.ServeStdio(s); err != nil {
		clipboardServer.stop()
		fmt.Fprintf(os.Stderr, "Fatal MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func (cs *ClipboardServer) readClipboardHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	format := "auto"
	if f := request.GetString("format", "auto"); f != "" {
		format = f
	}

	outputPath := request.GetString("output_path", "")
	chunkSize := 25000
	if cs := request.GetString("chunk_size", ""); cs != "" {
		if parsed, err := strconv.Atoi(cs); err == nil && parsed > 0 {
			chunkSize = parsed
		}
	}
	
	chunkIndex := -1
	if ci := request.GetString("chunk_index", ""); ci != "" {
		if parsed, err := strconv.Atoi(ci); err == nil && parsed >= 0 {
			chunkIndex = parsed
		}
	}

	// Handle file format with direct binary extraction on Windows
	if format == "file" || outputPath != "" {
		return cs.handleFileOutput(outputPath)
	}

	content, err := readClipboard()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read clipboard: %v", err)), nil
	}

	if content == "" {
		return mcp.NewToolResultText("Clipboard is empty"), nil
	}

	switch format {
	case "text":
		return cs.handleTextContent(content, chunkSize, chunkIndex)
	case "base64":
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		return cs.handleTextContent(encoded, chunkSize, chunkIndex)
	case "auto":
		if isProbablyText(content) {
			return cs.handleTextContent(content, chunkSize, chunkIndex)
		} else {
			return handleBinaryContent([]byte(content), cs)
		}
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown format: %s. Use 'text', 'base64', 'file', or 'auto'", format)), nil
	}
}

// handleTextContent handles text content with optional chunking
func (cs *ClipboardServer) handleTextContent(content string, chunkSize int, chunkIndex int) (*mcp.CallToolResult, error) {
	if len(content) <= chunkSize {
		// Content fits in a single response
		return mcp.NewToolResultText(content), nil
	}

	// Content is large - handle chunking
	totalChunks := (len(content) + chunkSize - 1) / chunkSize

	if chunkIndex < 0 {
		// Return metadata about chunks
		resourceURI := cs.getCurrentResourceURI()
		return mcp.NewToolResultText(fmt.Sprintf(
			"Clipboard content too large (%d bytes). Available in %d chunks of %d bytes each.\n"+
			"Use chunk_index parameter (0-%d) to get specific chunks, or access as resource: %s",
			len(content), totalChunks, chunkSize, totalChunks-1, resourceURI)), nil
	}

	if chunkIndex >= totalChunks {
		return mcp.NewToolResultError(fmt.Sprintf("Chunk index %d out of range (0-%d)", chunkIndex, totalChunks-1)), nil
	}

	// Return specific chunk
	start := chunkIndex * chunkSize
	end := start + chunkSize
	if end > len(content) {
		end = len(content)
	}

	chunk := content[start:end]
	metadata := fmt.Sprintf("Chunk %d/%d (%d-%d of %d bytes):\n", 
		chunkIndex+1, totalChunks, start, end-1, len(content))
	
	return mcp.NewToolResultText(metadata + chunk), nil
}

func (cs *ClipboardServer) startClipboardMonitoring(ctx context.Context) {
	// Set running state atomically
	if !atomic.CompareAndSwapInt32(&cs.running, 0, 1) {
		return // Already running
	}
	defer atomic.StoreInt32(&cs.running, 0)

	// Initialize clipboard package
	err := clipboard.Init()
	if err != nil {
		if os.Getenv("MCP_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "Failed to initialize clipboard: %v\n", err)
		}
		return
	}

	// Use event-driven clipboard monitoring
	textChanges := clipboard.Watch(ctx, clipboard.FmtText)
	imageChanges := clipboard.Watch(ctx, clipboard.FmtImage)

	// Get initial clipboard state
	if initialText := clipboard.Read(clipboard.FmtText); len(initialText) > 0 {
		cs.updateClipboard(string(initialText))
	} else if initialImage := clipboard.Read(clipboard.FmtImage); len(initialImage) > 0 {
		cs.updateClipboard(string(initialImage))
	}

	for {
		select {
		case <-ctx.Done():
			return // Graceful shutdown
		case textData, ok := <-textChanges:
			if !ok {
				if os.Getenv("MCP_DEBUG") == "1" {
					fmt.Fprintf(os.Stderr, "Text clipboard channel closed\n")
				}
				return
			}
			if len(textData) > 0 {
				cs.updateClipboard(string(textData))
			}
		case imageData, ok := <-imageChanges:
			if !ok {
				if os.Getenv("MCP_DEBUG") == "1" {
					fmt.Fprintf(os.Stderr, "Image clipboard channel closed\n")
				}
				return
			}
			if len(imageData) > 0 {
				cs.updateClipboard(string(imageData))
			}
		}
	}
}

// Helper function for min duration
func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func readClipboard() (string, error) {
	// Initialize clipboard access
	if err := clipboard.Init(); err != nil {
		return "", fmt.Errorf("failed to initialize clipboard: %v", err)
	}
	
	// Try text first
	if textData := clipboard.Read(clipboard.FmtText); len(textData) > 0 {
		return string(textData), nil
	}
	
	// Try image if no text
	if imageData := clipboard.Read(clipboard.FmtImage); len(imageData) > 0 {
		return string(imageData), nil
	}
	
	return "", nil
}


func getCleanupTTL() time.Duration {
	if ttlStr := os.Getenv("MCP_CLEANUP_TTL"); ttlStr != "" {
		if ttl, err := time.ParseDuration(ttlStr); err == nil {
			if os.Getenv("MCP_DEBUG") == "1" {
				fmt.Fprintf(os.Stderr, "Using custom cleanup TTL: %v\n", ttl)
			}
			return ttl
		} else {
			if os.Getenv("MCP_DEBUG") == "1" {
				fmt.Fprintf(os.Stderr, "Invalid MCP_CLEANUP_TTL format '%s', using default: %v\n", ttlStr, DefaultCleanupTTL)
			}
		}
	}
	return DefaultCleanupTTL
}

func cleanupExpiredFiles() error {
	tempDir := os.TempDir()
	ttl := getCleanupTTL()
	cutoffTime := time.Now().Add(-ttl)

	files, err := filepath.Glob(filepath.Join(tempDir, FilenamePrefix+"*"))
	if err != nil {
		return fmt.Errorf("failed to list temp files: %v", err)
	}

	var removed, errors int
	for _, filePath := range files {
		if shouldRemoveFile(filePath, cutoffTime) {
			if err := os.Remove(filePath); err != nil {
				errors++
				if os.Getenv("MCP_DEBUG") == "1" {
					fmt.Fprintf(os.Stderr, "Failed to remove expired file %s: %v\n", filePath, err)
				}
			} else {
				removed++
				if os.Getenv("MCP_DEBUG") == "1" {
					fmt.Fprintf(os.Stderr, "Removed expired file: %s\n", filePath)
				}
			}
		}
	}

	if os.Getenv("MCP_DEBUG") == "1" && (removed > 0 || errors > 0) {
		fmt.Fprintf(os.Stderr, "Cleanup complete: %d removed, %d errors\n", removed, errors)
	}

	return nil
}

func shouldRemoveFile(filePath string, cutoffTime time.Time) bool {
	filename := filepath.Base(filePath)

	// Extract timestamp from filename: mcp-clip-{timestamp}-{hash}.{ext}
	if !strings.HasPrefix(filename, FilenamePrefix) {
		return false
	}

	parts := strings.Split(strings.TrimPrefix(filename, FilenamePrefix), "-")
	if len(parts) < 2 {
		// Old format without timestamp, remove it
		return true
	}

	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		// Invalid timestamp format, remove it
		return true
	}

	fileTime := time.Unix(timestamp, 0)
	return fileTime.Before(cutoffTime)
}

func (cs *ClipboardServer) getCurrentResourceURI() string {
	// Register the resource if not already done
	if !cs.resourceRegistered && cs.mcpServer != nil {
		cs.registerCurrentResource()
	}
	return CurrentResourceURI
}

func (cs *ClipboardServer) registerCurrentResource() {
	resource := mcp.Resource{
		URI:         CurrentResourceURI,
		Name:        "current-clipboard",
		Description: "Current clipboard content",
		MIMEType:    "application/octet-stream", // Will be determined dynamically
	}

	// Create resource handler that reads current clipboard
	handler := func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		content, err := readClipboard()
		if err != nil {
			return nil, fmt.Errorf("failed to read current clipboard: %v", err)
		}

		if content == "" {
			return []mcp.ResourceContents{&mcp.TextResourceContents{
				URI:      CurrentResourceURI,
				MIMEType: "text/plain",
				Text:     "",
			}}, nil
		}

		data := []byte(content)
		
		// Determine content type and MIME type
		if isProbablyText(content) {
			return []mcp.ResourceContents{&mcp.TextResourceContents{
				URI:      CurrentResourceURI,
				MIMEType: "text/plain",
				Text:     content,
			}}, nil
		} else {
			// Binary content (likely image)
			isImage, imageType := detectImageType(data)
			var mimeType string
			if isImage {
				switch imageType {
				case "png":
					mimeType = "image/png"
				case "jpg", "jpeg":
					mimeType = "image/jpeg"
				case "gif":
					mimeType = "image/gif"
				case "webp":
					mimeType = "image/webp"
				case "bmp":
					mimeType = "image/bmp"
				default:
					mimeType = "application/octet-stream"
				}
			} else {
				mimeType = "application/octet-stream"
			}

			encoded := base64.StdEncoding.EncodeToString(data)
			return []mcp.ResourceContents{&mcp.BlobResourceContents{
				URI:      CurrentResourceURI,
				MIMEType: mimeType,
				Blob:     encoded,
			}}, nil
		}
	}

	// Add resource to server
	cs.mcpServer.AddResource(resource, handler)
	cs.resourceRegistered = true

	if os.Getenv("MCP_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "Registered current clipboard resource: %s\n", CurrentResourceURI)
	}
}

func saveToTempFile(data []byte, extension string, cs *ClipboardServer) (string, error) {
	// Clean up expired files before creating new ones
	if err := cleanupExpiredFiles(); err != nil {
		// Log error but don't fail - cleanup is best effort
		if os.Getenv("MCP_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "Cleanup warning: %v\n", err)
		}
	}

	hash := md5.Sum(data)
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("mcp-clip-%d-%s.%s", timestamp, hex.EncodeToString(hash[:]), extension)
	tempDir := os.TempDir()
	filePath := filepath.Join(tempDir, filename)

	// FIXED: Use atomic file creation with O_CREATE|O_EXCL to prevent TOCTOU race
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// File already exists, return the path
			return filePath, nil
		}
		return "", fmt.Errorf("failed to create temp file %s (extension: %s, size: %d bytes, tempDir: %s): %v",
			filePath, extension, len(data), tempDir, err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		// Clean up partially created file
		os.Remove(filePath)
		return "", fmt.Errorf("failed to write temp file %s (extension: %s, size: %d bytes): %v",
			filePath, extension, len(data), err)
	}

	if os.Getenv("MCP_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "Created temp file: %s (%d bytes)\n", filePath, len(data))
	}

	// Track file for session cleanup if server instance provided
	if cs != nil {
		cs.addSessionFile(filePath)
	}

	return filePath, nil
}

func handleBinaryContent(data []byte, cs *ClipboardServer) (*mcp.CallToolResult, error) {
	isImage, imageType := detectImageType(data)

	if isImage {
		resourceURI := cs.getCurrentResourceURI()
		return mcp.NewToolResultText(fmt.Sprintf("Clipboard image content (%s, %d bytes). Available as resource: %s", imageType, len(data), resourceURI)), nil
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	const maxDirectOutput = 25000

	if len(encoded) > maxDirectOutput {
		resourceURI := cs.getCurrentResourceURI()
		return mcp.NewToolResultText(fmt.Sprintf("Clipboard binary content too large (%d bytes base64). Available as resource: %s", len(encoded), resourceURI)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Clipboard binary content (base64 encoded):\n%s", encoded)), nil
}

func detectImageType(data []byte) (bool, string) {
	if len(data) < 8 {
		return false, ""
	}

	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return true, "png"
	}

	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true, "jpg"
	}

	if len(data) >= 6 && string(data[0:6]) == "GIF87a" || string(data[0:6]) == "GIF89a" {
		return true, "gif"
	}

	if len(data) >= 12 && string(data[8:12]) == "WEBP" {
		return true, "webp"
	}

	if data[0] == 0x42 && data[1] == 0x4D {
		return true, "bmp"
	}

	return false, ""
}

func isProbablyText(content string) bool {
	if len(content) == 0 {
		return true
	}

	textChars := 0
	for _, r := range content {
		if r >= 32 && r <= 126 || r == '\n' || r == '\r' || r == '\t' {
			textChars++
		}
	}

	return float64(textChars)/float64(len(content)) > 0.8
}

func isRunningFromCLI() bool {
	if fileInfo, err := os.Stdin.Stat(); err == nil {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return true
}

func printUsage() {
	fmt.Printf(`USAGE:
    This MCP server provides clipboard access for MCP clients like Claude Desktop.
    
    For direct testing:
    %s --help           Show this help message
    %s test             Test clipboard functionality
    %s version          Show version information
    
    For MCP client usage:
    1. Build the server:
       go build -o mcp-clip
    
    2. Add to your MCP client configuration:
       Claude Desktop: Add to claude_desktop_config.json
       {
         "mcpServers": {
           "mcp-clip": {
             "command": "/path/to/mcp-clip"
           }
         }
       }
    
    3. Start your MCP client (Claude Desktop, etc.)
    
    Available Tools:
    - read_clipboard: Read clipboard content (text/images as base64)
    
    Features:
    - Automatic clipboard monitoring with notifications
    - Support for text and binary clipboard content
    - Base64 encoding for binary data (like images)
    - Smart content type detection
    
    Environment Variables:
    - MCP_DEBUG=1: Enable debug logging
    
    For more information about MCP:
    https://modelcontextprotocol.io/
`, os.Args[0], os.Args[0], os.Args[0])
}

// handleFileOutput extracts clipboard content directly to a file, using platform-specific methods
func (cs *ClipboardServer) handleFileOutput(outputPath string) (*mcp.CallToolResult, error) {
	if runtime.GOOS == "windows" {
		return cs.handleWindowsFileOutput(outputPath)
	}
	
	// Fallback for non-Windows: use x/clipboard and save to file
	content, err := readClipboard()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read clipboard: %v", err)), nil
	}
	
	if content == "" {
		return mcp.NewToolResultText("Clipboard is empty"), nil
	}
	
	var filePath string
	var data []byte
	
	if isProbablyText(content) {
		// Text content
		data = []byte(content)
		if outputPath != "" {
			filePath = outputPath
		} else {
			var err error
			filePath, err = saveToTempFile(data, "txt", cs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to save text file: %v", err)), nil
			}
		}
	} else {
		// Binary content
		data = []byte(content)
		isImage, imageType := detectImageType(data)
		ext := "bin"
		if isImage {
			ext = imageType
		}
		
		if outputPath != "" {
			filePath = outputPath
		} else {
			var err error
			filePath, err = saveToTempFile(data, ext, cs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to save binary file: %v", err)), nil
			}
		}
	}
	
	// Write file if outputPath was specified
	if outputPath != "" {
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to write file %s: %v", filePath, err)), nil
		}
	}
	
	return mcp.NewToolResultText(fmt.Sprintf("Clipboard content saved to: %s (%d bytes)", filePath, len(data))), nil
}

// handleWindowsFileOutput uses PowerShell to extract clipboard content directly to files
func (cs *ClipboardServer) handleWindowsFileOutput(outputPath string) (*mcp.CallToolResult, error) {
	// First check what's in the clipboard using PowerShell
	checkScript := `
$clip = Get-Clipboard -Raw -ErrorAction SilentlyContinue
if ($clip) {
    Write-Output "text:$($clip.Length)"
    exit 0
}

# Check for image
try {
    Add-Type -AssemblyName System.Windows.Forms
    $image = [System.Windows.Forms.Clipboard]::GetImage()
    if ($image) {
        Write-Output "image:$($image.Width)x$($image.Height)"
        exit 0
    }
} catch {}

# Check for files
try {
    $files = [System.Windows.Forms.Clipboard]::GetFileDropList()
    if ($files -and $files.Count -gt 0) {
        Write-Output "files:$($files.Count)"
        exit 0
    }
} catch {}

Write-Output "empty"
`
	
	cmd := exec.Command("powershell", "-NoProfile", "-Command", checkScript)
	output, err := cmd.Output()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to check clipboard: %v", err)), nil
	}
	
	clipboardType := strings.TrimSpace(string(output))
	
	if clipboardType == "empty" {
		return mcp.NewToolResultText("Clipboard is empty"), nil
	}
	
	var filePath string
	var fileSize int64
	
	if strings.HasPrefix(clipboardType, "text:") {
		// Handle text content
		
		script := `Get-Clipboard -Raw`
		if outputPath != "" {
			script = fmt.Sprintf(`Get-Clipboard -Raw | Out-File -FilePath "%s" -Encoding UTF8`, outputPath)
			filePath = outputPath
		} else {
			// Create temp file
			tempFile, err := cs.createTempFile("txt")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
			}
			filePath = tempFile
			script = fmt.Sprintf(`Get-Clipboard -Raw | Out-File -FilePath "%s" -Encoding UTF8`, filePath)
		}
		
		cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to save text clipboard: %v", err)), nil
		}
		
		if info, err := os.Stat(filePath); err == nil {
			fileSize = info.Size()
		}
		
		return mcp.NewToolResultText(fmt.Sprintf("Text clipboard content saved to: %s (%d bytes)", filePath, fileSize)), nil
		
	} else if strings.HasPrefix(clipboardType, "image:") {
		// Handle image content
		dimensions := strings.TrimPrefix(clipboardType, "image:")
		
		if outputPath != "" {
			filePath = outputPath
		} else {
			// Create temp file
			tempFile, err := cs.createTempFile("png")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
			}
			filePath = tempFile
		}
		
		// PowerShell script to save image
		script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$image = [System.Windows.Forms.Clipboard]::GetImage()
if ($image) {
    $image.Save("%s", [System.Drawing.Imaging.ImageFormat]::Png)
    Write-Output "Saved"
} else {
    Write-Output "Failed"
}
`, filePath)
		
		cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
		output, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(output)) != "Saved" {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to save image clipboard: %v", err)), nil
		}
		
		if info, err := os.Stat(filePath); err == nil {
			fileSize = info.Size()
		}
		
		return mcp.NewToolResultText(fmt.Sprintf("Image clipboard content (%s) saved to: %s (%d bytes)", dimensions, filePath, fileSize)), nil
		
	} else if strings.HasPrefix(clipboardType, "files:") {
		// Handle file list
		count := strings.TrimPrefix(clipboardType, "files:")
		
		script := `
Add-Type -AssemblyName System.Windows.Forms
$files = [System.Windows.Forms.Clipboard]::GetFileDropList()
foreach ($file in $files) {
    Write-Output $file
}
`
		
		cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
		output, err := cmd.Output()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get file list: %v", err)), nil
		}
		
		fileList := strings.TrimSpace(string(output))
		return mcp.NewToolResultText(fmt.Sprintf("Clipboard contains %s files:\n%s", count, fileList)), nil
	}
	
	return mcp.NewToolResultError("Unknown clipboard content type"), nil
}

// createTempFile creates a temp file and tracks it for cleanup
func (cs *ClipboardServer) createTempFile(extension string) (string, error) {
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("mcp-clip-%d.%s", timestamp, extension)
	tempDir := os.TempDir()
	filePath := filepath.Join(tempDir, filename)
	
	// Create empty file
	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	file.Close()
	
	// Track for cleanup
	cs.addSessionFile(filePath)
	
	return filePath, nil
}

func handleTestCommand() {
	fmt.Println("Testing clipboard functionality...")

	content, err := readClipboard()
	if err != nil {
		fmt.Printf("❌ Failed to read clipboard: %v\n", err)
		return
	}

	if content == "" {
		fmt.Println("📋 Clipboard is empty")
		return
	}

	fmt.Printf("📋 Clipboard content detected (%d bytes)\n", len(content))

	if isProbablyText(content) {
		fmt.Println("📝 Content type: Text")
		if len(content) <= 100 {
			fmt.Printf("📄 Content: %s\n", content)
		} else {
			fmt.Printf("📄 Content preview: %s...\n", content[:100])
		}
	} else {
		fmt.Println("🖼️  Content type: Binary (possibly image)")
		fmt.Printf("📦 Base64 preview: %s...\n", base64.StdEncoding.EncodeToString([]byte(content))[:50])
	}

	fmt.Println("✅ Clipboard test completed successfully")
}

