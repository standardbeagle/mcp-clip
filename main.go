package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ClipboardServer struct {
	lastClipboardContent string
	lastClipboardTime    time.Time
	mu                   sync.RWMutex
	notificationChan     chan string
}

func NewClipboardServer() *ClipboardServer {
	return &ClipboardServer{
		notificationChan: make(chan string, 10),
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

	go clipboardServer.startClipboardMonitoring(s)

	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
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

	switch format {
	case "text":
		return mcp.NewToolResultText(content), nil
	case "base64":
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		return mcp.NewToolResultText(fmt.Sprintf("Base64 encoded clipboard content:\n%s", encoded)), nil
	case "auto":
		if isProbablyText(content) {
			return mcp.NewToolResultText(fmt.Sprintf("Clipboard text content:\n%s", content)), nil
		} else {
			encoded := base64.StdEncoding.EncodeToString([]byte(content))
			return mcp.NewToolResultText(fmt.Sprintf("Clipboard binary content (base64 encoded):\n%s", encoded)), nil
		}
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown format: %s. Use 'text', 'base64', or 'auto'", format)), nil
	}
}

func (cs *ClipboardServer) startClipboardMonitoring(s *server.MCPServer) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			content, err := readClipboard()
			if err != nil {
				continue
			}

			cs.mu.Lock()
			if content != cs.lastClipboardContent && content != "" {
				cs.lastClipboardContent = content
				cs.lastClipboardTime = time.Now()
				cs.mu.Unlock()

				contentType := "text"
				if !isProbablyText(content) {
					contentType = "binary"
				}

				fmt.Printf("📋 Clipboard updated: %s content (%d bytes) - %s\n", 
					contentType, len(content), getContentPreview(content))
			} else {
				cs.mu.Unlock()
			}
		}
	}
}

func readClipboard() (string, error) {
	if isWSL2() {
		return readClipboardWSL2()
	}
	return clipboard.ReadAll()
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

func readClipboardWSL2() (string, error) {
	powershellPath := findPowerShell()
	if powershellPath == "" {
		return "", fmt.Errorf("PowerShell not found - required for WSL2 clipboard access")
	}
	
	textCmd := exec.Command(powershellPath, "-Command", "Get-Clipboard -Raw")
	textOutput, textErr := textCmd.Output()
	
	if textErr == nil && len(textOutput) > 0 {
		content := strings.TrimSpace(string(textOutput))
		if content != "" {
			return content, nil
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
			return content, nil
		}
	}
	
	return "", nil
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

func getContentPreview(content string) string {
	if len(content) <= 50 {
		return content
	}
	
	if isProbablyText(content) {
		return content[:50] + "..."
	}
	
	return fmt.Sprintf("[Binary data, %d bytes]", len(content))
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