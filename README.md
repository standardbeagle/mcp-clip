# MCP Clipboard Server

A Model Context Protocol (MCP) server that provides clipboard access for MCP clients like Claude Desktop. This server allows reading clipboard content including text and images, with automatic monitoring and notifications when clipboard content changes.

## Features

- 📋 **Clipboard Reading**: Read current clipboard content
- 🔄 **Auto-monitoring**: Automatic clipboard change detection
- 📝 **Text Support**: Full text clipboard support
- 🖼️ **Image Support**: Binary content (images) encoded as base64
- 🎯 **Smart Detection**: Automatic content type detection
- 📢 **Notifications**: Real-time clipboard change notifications (console output)

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

## Environment Variables

- `MCP_DEBUG=1`: Enable debug logging (if implemented)

## Security & Privacy

- The server only reads clipboard content when explicitly requested via the `read_clipboard` tool
- Clipboard monitoring only detects changes but doesn't read content until requested
- No clipboard content is stored or persisted
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