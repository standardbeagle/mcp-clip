# MCP Clipboard Server

A Model Context Protocol (MCP) server that provides clipboard access for MCP clients like Claude Desktop. This server allows reading clipboard content including text and images, with automatic monitoring and notifications when clipboard content changes.

## Features

- 📋 **Clipboard Reading**: Read current clipboard content
- 🔄 **Auto-monitoring**: Automatic clipboard change detection
- 📝 **Text Support**: Full text clipboard support
- 🖼️ **Image Support**: Binary content (images) with smart format detection
- 🎯 **Smart Detection**: Automatic content type detection
- 📁 **Large Content Handling**: Automatic temp file creation for content >25KB
- 🧹 **Automatic Cleanup**: TTL-based cleanup of temp files
- 🔒 **Secure**: Restrictive file permissions and safe cleanup

## Installation

### Prerequisites

- Go 1.21 or later
- A system with clipboard support (Linux, macOS, Windows)

### Build from Source

```bash
git clone <repository-url>
cd mcp-clip
go build -o mcp-clip
```

## Usage

### MCP Client Configuration

Add to your MCP client configuration (e.g., Claude Desktop):

```json
{
  "mcpServers": {
    "mcp-clip": {
      "command": "/path/to/mcp-clip"
    }
  }
}
```

### Command Line Testing

```bash
# Test clipboard functionality
./mcp-clip test

# Show help
./mcp-clip --help

# Show version
./mcp-clip version
```

## Tools

### read_clipboard

Read the current clipboard content with support for different formats.

**Parameters:**
- `format` (optional): Format to return content in
  - `"text"` - Return as plain text
  - `"base64"` - Return as base64 encoded string
  - `"auto"` (default) - Automatically detect and format appropriately

**Examples:**
```json
{
  "tool": "read_clipboard",
  "arguments": {
    "format": "auto"
  }
}
```

## Clipboard Monitoring

The server automatically monitors clipboard changes every 500ms and provides console notifications when content changes. This helps track clipboard activity without actively reading the content until requested.

## Content Type Detection

The server intelligently detects content types:
- **Text**: Human-readable text content
- **Binary**: Images, files, or other binary data (encoded as base64)

## Platform Support

- ✅ **Linux**: X11 and Wayland support
- ✅ **macOS**: Native clipboard support
- ✅ **Windows**: Native clipboard support

## Large Content & Temp Files

When clipboard content exceeds 25KB, the server automatically saves it to temporary files instead of returning it directly (due to MCP protocol limitations).

### Temp File Behavior:
- **Location**: System temp directory (`/tmp` on Unix, `%TEMP%` on Windows)
- **Naming**: `mcp-clip-{timestamp}-{hash}.{extension}`
- **Permissions**: 0600 (owner read/write only)
- **Automatic Cleanup**: Files older than 1 hour are automatically removed

### Cleanup Strategy:
- **On-demand**: Cleanup runs before creating new temp files
- **Startup**: Orphaned files from previous sessions are cleaned
- **Graceful shutdown**: Session files are cleaned on SIGTERM/SIGINT
- **Safe**: Only removes mcp-clip files, preserves other applications' files

## Environment Variables

- `MCP_DEBUG=1`: Enable debug logging for troubleshooting
- `MCP_CLEANUP_TTL=1h`: File age threshold for cleanup (default: 1 hour)
  - Accepts Go duration format: `30m`, `2h`, `1h30m`, etc.

## Security & Privacy

- The server only reads clipboard content when explicitly requested via the `read_clipboard` tool
- Clipboard monitoring only detects changes but doesn't read content until requested
- Temp files use restrictive permissions (0600) - only readable by file owner
- Automatic cleanup prevents temp file accumulation
- No clipboard content is stored or persisted beyond temp files
- No network communication beyond MCP protocol

## Use Cases

- **VS Code Integration**: Paste images into Claude conversations in VS Code
- **Clipboard History**: Track when clipboard content changes
- **Multi-format Support**: Handle both text and binary clipboard content
- **Development Tools**: Debug clipboard-related functionality

## Troubleshooting

### Common Issues

1. **Clipboard is empty**: Make sure you have copied something to the clipboard
2. **Permission denied**: Ensure the application has clipboard access permissions
3. **Build errors**: Make sure Go 1.21+ is installed and dependencies are available

### Testing

```bash
# Copy some text to clipboard then test
echo "Hello World" | xclip -selection clipboard  # Linux
echo "Hello World" | pbcopy                      # macOS

# Then test
./mcp-clip test
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## License

This project is licensed under the MIT License.