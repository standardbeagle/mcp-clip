# MCP Clipboard Server

[![Go Version](https://img.shields.io/badge/go-1.24+-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![MCP Compatible](https://img.shields.io/badge/MCP-Compatible-green.svg)](https://modelcontextprotocol.io/)

A high-performance, lock-free [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that provides clipboard access for AI assistants. Specifically designed to solve the VSCode + WSL2 image clipboard limitation when using Claude.

## 🎯 Problem Solved

**VSCode + WSL2 + Claude Image Clipboard Issue**: When running Claude in VSCode on Windows with WSL2, pasting images from the clipboard doesn't work due to the sandboxed environment. This MCP server provides a bridge to access Windows clipboard content (including images) from within the WSL2 environment.

## ✨ Features

- 🔒 **Lock-free concurrent design** - High performance with zero race conditions
- 🖼️ **Image clipboard support** - PNG, JPEG, GIF, WebP, BMP detection and handling
- 🛡️ **WSL2 compatibility** - Seamless Windows clipboard access from WSL2
- 📁 **Large content handling** - Automatic temp file creation for content >25KB
- 🔄 **Background monitoring** - Watches the clipboard for changes so the current content is always ready to read
- 🧹 **Smart cleanup** - TTL-based temp file management
- 🚀 **Race condition free** - Comprehensive atomic operations and CAS loops
- 🔍 **Rich debugging** - Detailed error context and optional debug logging

## 🚀 Quick Start

### NPM Installation (Recommended)

```bash
npm install -g @standardbeagle/mcp-clip
```

### Go Installation

```bash
go install github.com/standardbeagle/mcp-clip@latest
```

### Manual Installation

```bash
git clone https://github.com/standardbeagle/mcp-clip.git
cd mcp-clip
go build -o mcp-clip
sudo cp mcp-clip /usr/local/bin/
```

## 📋 Claude Desktop Configuration

Add to your Claude Desktop `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "clipboard": {
      "command": "mcp-clip",
      "args": []
    }
  }
}
```

**Config file locations:**
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux**: `~/.config/Claude/claude_desktop_config.json`

## 🔧 VSCode + WSL2 Setup

### 1. Install in WSL2
```bash
# In your WSL2 terminal
npm install -g @standardbeagle/mcp-clip
```

### 2. Configure Claude Code Extension
Add to your VSCode settings or workspace `.vscode/settings.json`:

```json
{
  "claude-dev.mcpServers": {
    "clipboard": {
      "command": "mcp-clip",
      "args": []
    }
  }
}
```

### 3. WSL2 PowerShell Access
Ensure PowerShell is accessible from WSL2:
```bash
# Test PowerShell access
/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe -Command "Get-Clipboard"
```

## 🛠️ Available Tools

### `read_clipboard`
Reads current clipboard content with automatic format detection.

**Example usage in Claude:**
> "Read my clipboard and analyze the content"

**Supported formats:**
- Plain text
- Images (PNG, JPEG, GIF, WebP, BMP)
- Binary data (base64 encoded)

**Large content handling:**
- Content over 25KB (the default `chunk_size`) is returned in numbered chunks, or as a
  pointer to the `mcp-clip://current` resource, instead of one oversized response
- Pass `format: "file"` or `output_path` to write the clipboard content to disk directly
  (images always go through this path on Windows, via PowerShell)
- File paths are returned in the tool result for external access

## 📊 The `mcp-clip://current` Resource

The server also registers a `mcp-clip://current` resource that always reflects the current
clipboard content, so a client can read it directly instead of calling `read_clipboard`:

```json
{
  "uri": "mcp-clip://current"
}
```

The server tracks clipboard changes internally as they happen, but does not yet push
`notifications/resources/updated` events to the client — polling `read_clipboard` or the
resource is still required to see new content.

## ⚙️ Configuration

### Environment Variables

- `MCP_DEBUG=1` - Enable detailed debug logging
- `MCP_CLEANUP_TTL=2h` - Set temp file cleanup TTL (default: 1h)

### Debug Mode
```bash
MCP_DEBUG=1 mcp-clip
```

Shows detailed information about:
- Clipboard monitoring status
- Temp file creation and cleanup
- WSL2 PowerShell integration
- Race condition fallbacks (extremely rare)

## 🧪 Testing

### Test Installation
```bash
mcp-clip test
```

### Development Testing
```bash
# Run tests with race detector
go test -race -v ./...

# Build with race detector
go build -race .
```

## 🏗️ Architecture

### Lock-Free Design
- **Atomic operations** for all shared state
- **Compare-and-swap loops** with retry limits
- **Zero mutexes** in hot paths
- **Race-condition free** under all loads

### Concurrency Safety
- Atomic clipboard state updates
- Thread-safe session file tracking  
- Graceful shutdown with cleanup
- TOCTOU-safe temp file creation

### Platform Integration
- **WSL2**: PowerShell bridge for Windows clipboard
- **Linux**: Direct clipboard integration via golang.design/x/clipboard
- **macOS**: Native clipboard support
- **Windows**: Native clipboard support

## 🔧 Troubleshooting

### WSL2 Issues

**PowerShell not found:**
```bash
# Install Windows PowerShell in WSL2
sudo apt update && sudo apt install powershell
```

**Permission denied:**
```bash
# Add Windows PowerShell to PATH
echo 'export PATH="/mnt/c/Windows/System32/WindowsPowerShell/v1.0:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

**Clipboard empty in WSL2:**
```bash
# Test Windows clipboard access
powershell.exe "Get-Clipboard" 
```

### General Issues

**MCP connection failed:**
- Verify `mcp-clip` is in your PATH
- Check Claude Desktop config JSON syntax
- Restart Claude Desktop after config changes

**Large images not working:**
- Check available disk space in temp directory
- Verify temp directory permissions
- Enable debug mode: `MCP_DEBUG=1`

**Performance issues:**
- Monitor with debug mode enabled
- Check for antivirus interference
- Verify WSL2 resource allocation

## 📝 Development

### Building from Source
```bash
git clone https://github.com/standardbeagle/mcp-clip.git
cd mcp-clip
go mod download
go build -race .
```

### Running Tests
```bash
go test -race -v ./...
```

### Contributing
1. Fork the repository
2. Create a feature branch
3. Run tests: `go test -race -v ./...`
4. Submit a pull request

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Model Context Protocol](https://modelcontextprotocol.io/) by Anthropic
- [golang.design/x/clipboard](https://github.com/golang-design/clipboard) for cross-platform clipboard access
- [mcp-go](https://github.com/mark3labs/mcp-go) for Go MCP implementation

## 🔗 Links

- [GitHub Repository](https://github.com/standardbeagle/mcp-clip)
- [NPM Package](https://www.npmjs.com/package/@standardbeagle/mcp-clip)
- [Model Context Protocol](https://modelcontextprotocol.io/)
- [Claude Desktop](https://claude.ai/desktop)