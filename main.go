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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	DefaultCleanupTTL = 1 * time.Hour
	FilenamePrefix    = "mcp-clip-"
)

type clipboardState struct {
	content string
	time    time.Time
}

type ClipboardServer struct {
	lastClipboard atomic.Value // stores clipboardState
	running       int32        // atomic flag for monitoring state
	cancel        context.CancelFunc
}

func NewClipboardServer() *ClipboardServer {
	cs := &ClipboardServer{}
	cs.lastClipboard.Store(clipboardState{})
	return cs
}

func (cs *ClipboardServer) updateClipboard(content string) bool {
	if content == "" {
		return false
	}
	
	// Get current state
	currentState, _ := cs.getLastClipboard()
	
	// Only update if content has changed
	if content != currentState {
		cs.lastClipboard.Store(clipboardState{
			content: content,
			time:    time.Now(),
		})
		return true
	}
	return false
}

func (cs *ClipboardServer) getLastClipboard() (string, time.Time) {
	if state, ok := cs.lastClipboard.Load().(clipboardState); ok {
		return state.content, state.time
	}
	return "", time.Time{}
}

func (cs *ClipboardServer) stop() {
	if atomic.CompareAndSwapInt32(&cs.running, 1, 0) {
		if cs.cancel != nil {
			cs.cancel()
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

	s := server.NewMCPServer(
		"mcp-clip",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	readClipboardTool := mcp.NewTool("read_clipboard",
		mcp.WithDescription("Read the current clipboard content, supporting text and images"),
		mcp.WithString("format",
			mcp.Description("Format to return clipboard content in: 'text', 'base64', or 'auto' (default)"),
		),
	)

	s.AddTool(readClipboardTool, clipboardServer.readClipboardHandler)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	clipboardServer.cancel = cancel

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

	content, err := readClipboard()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read clipboard: %v", err)), nil
	}

	if content == "" {
		return mcp.NewToolResultText("Clipboard is empty"), nil
	}

	const maxDirectOutput = 25000
	
	switch format {
	case "text":
		if len(content) > maxDirectOutput {
			filePath, err := saveToTempFile([]byte(content), "txt")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to save large content to temp file: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Clipboard text content too large (%d bytes). Saved to: %s", len(content), filePath)), nil
		}
		return mcp.NewToolResultText(content), nil
	case "base64":
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		if len(encoded) > maxDirectOutput {
			filePath, err := saveToTempFile([]byte(encoded), "b64")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to save large base64 content to temp file: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Base64 encoded clipboard content too large (%d bytes). Saved to: %s", len(encoded), filePath)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Base64 encoded clipboard content:\n%s", encoded)), nil
	case "auto":
		if isProbablyText(content) {
			if len(content) > maxDirectOutput {
				filePath, err := saveToTempFile([]byte(content), "txt")
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to save large text content to temp file: %v", err)), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("Clipboard text content too large (%d bytes). Saved to: %s", len(content), filePath)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Clipboard text content:\n%s", content)), nil
		} else {
			return handleBinaryContent([]byte(content))
		}
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown format: %s. Use 'text', 'base64', or 'auto'", format)), nil
	}
}

func (cs *ClipboardServer) startClipboardMonitoring(ctx context.Context) {
	// Set running state atomically
	if !atomic.CompareAndSwapInt32(&cs.running, 0, 1) {
		return // Already running
	}
	defer atomic.StoreInt32(&cs.running, 0)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return // Graceful shutdown
		case <-ticker.C:
			content, err := readClipboard()
			if err != nil {
				// In debug mode, we could log this error
				if os.Getenv("MCP_DEBUG") == "1" {
					fmt.Fprintf(os.Stderr, "Clipboard read error: %v\n", err)
				}
				continue
			}

			// Use lock-free update
			cs.updateClipboard(content)
		}
	}
}

func readClipboard() (string, error) {
	if isWSL2() {
		data, err := readClipboardDataWSL2()
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return clipboard.ReadAll()
}

func readClipboardDataWSL2() ([]byte, error) {
	powershellPath := findPowerShell()
	if powershellPath == "" {
		return nil, fmt.Errorf("PowerShell not found - required for WSL2 clipboard access")
	}
	
	textCmd := exec.Command(powershellPath, "-Command", "Get-Clipboard -Raw")
	textOutput, textErr := textCmd.Output()
	
	if textErr == nil && len(textOutput) > 0 {
		content := strings.TrimSpace(string(textOutput))
		if content != "" {
			return []byte(content), nil
		}
	}
	
	imageCmd := exec.Command(powershellPath, "-Command", `
		$image = Get-Clipboard -Format Image
		if ($image -ne $null) {
			$ms = New-Object System.IO.MemoryStream
			$image.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
			[Convert]::ToBase64String($ms.ToArray())
		}
	`)
	imageOutput, imageErr := imageCmd.Output()
	
	if imageErr == nil && len(imageOutput) > 0 {
		content := strings.TrimSpace(string(imageOutput))
		if content != "" {
			data, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64 image data: %v", err)
			}
			return data, nil
		}
	}
	
	return []byte{}, nil
}

func isWSL2() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	
	if _, err := os.Stat("/proc/version"); err != nil {
		return false
	}
	
	content, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	
	return strings.Contains(strings.ToLower(string(content)), "microsoft") || 
		   strings.Contains(strings.ToLower(string(content)), "wsl")
}

func findPowerShell() string {
	powershellPaths := []string{
		"/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
		"/mnt/c/WINDOWS/System32/WindowsPowerShell/v1.0/powershell.exe",
		"/mnt/c/windows/system32/windowspowershell/v1.0/powershell.exe",
	}
	
	for _, path := range powershellPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	
	cmd := exec.Command("which", "powershell.exe")
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}
	
	return ""
}

func getCleanupTTL() time.Duration {
	if ttlStr := os.Getenv("MCP_CLEANUP_TTL"); ttlStr != "" {
		if ttl, err := time.ParseDuration(ttlStr); err == nil {
			return ttl
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

func saveToTempFile(data []byte, extension string) (string, error) {
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
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		err = os.WriteFile(filePath, data, 0600) // More restrictive permissions
		if err != nil {
			return "", fmt.Errorf("failed to write temp file %s: %v", filePath, err)
		}
		
		if os.Getenv("MCP_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "Created temp file: %s (%d bytes)\n", filePath, len(data))
		}
	}
	
	return filePath, nil
}

func handleBinaryContent(data []byte) (*mcp.CallToolResult, error) {
	isImage, imageType := detectImageType(data)
	
	if isImage {
		filePath, err := saveToTempFile(data, imageType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to save image to temp file: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Clipboard image content (%s, %d bytes). Saved to: %s", imageType, len(data), filePath)), nil
	}
	
	encoded := base64.StdEncoding.EncodeToString(data)
	const maxDirectOutput = 25000
	
	if len(encoded) > maxDirectOutput {
		filePath, err := saveToTempFile([]byte(encoded), "b64")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to save large binary content to temp file: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Clipboard binary content too large (%d bytes base64). Saved to: %s", len(encoded), filePath)), nil
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

func handleTestCommand() {
	fmt.Println("Testing clipboard functionality...")
	
	// Test cleanup functionality
	fmt.Println("\n🧹 Testing cleanup functionality...")
	if err := cleanupExpiredFiles(); err != nil {
		fmt.Printf("❌ Cleanup failed: %v\n", err)
	} else {
		fmt.Println("✅ Cleanup completed")
	}
	
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
	
	// Test temp file creation if content is large
	if len(content) > 25000 {
		fmt.Println("📁 Testing temp file creation...")
		filePath, err := saveToTempFile([]byte(content), "txt")
		if err != nil {
			fmt.Printf("❌ Failed to create temp file: %v\n", err)
		} else {
			fmt.Printf("✅ Created temp file: %s\n", filePath)
		}
	}
	
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