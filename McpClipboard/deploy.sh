#!/bin/bash

# Deploy script for MCP Clipboard C# Server
# Builds on WSL and deploys to Windows accessible location

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_DIR="$(pwd)"
BUILD_CONFIG="Release"
TARGET_FRAMEWORK="net9.0-windows"
WINDOWS_DEPLOY_DIR="/mnt/c/Tools/mcp-clipboard"
EXE_NAME="McpClipboard.exe"

echo -e "${BLUE}🔧 MCP Clipboard C# Server Deployment${NC}"
echo "================================================="

# Check if we're in the right directory
if [[ ! -f "McpClipboard.csproj" ]]; then
    echo -e "${RED}❌ Error: McpClipboard.csproj not found${NC}"
    echo "Please run this script from the McpClipboard project directory"
    exit 1
fi

# Step 1: Clean previous builds and obj directory
echo -e "${YELLOW}🧹 Cleaning previous builds...${NC}"
rm -rf obj/ bin/ 2>/dev/null || true
echo -e "${GREEN}✅ Cleaned obj/ and bin/ directories${NC}"

# Step 2: Restore dependencies for win-x64 runtime
echo -e "${YELLOW}📦 Restoring dependencies for win-x64...${NC}"
powershell.exe -c "dotnet restore -r win-x64" || {
    echo -e "${RED}❌ Failed to restore dependencies${NC}"
    exit 1
}

# Step 3: Publish as self-contained (includes all dependencies)
echo -e "${YELLOW}🔨 Publishing self-contained executable...${NC}"
powershell.exe -c "dotnet publish -c $BUILD_CONFIG -r win-x64 --self-contained true --no-restore" || {
    echo -e "${RED}❌ Publish failed${NC}"
    exit 1
}

# Step 4: Create Windows deployment directory
echo -e "${YELLOW}📁 Creating deployment directory...${NC}"
if [[ ! -d "$WINDOWS_DEPLOY_DIR" ]]; then
    mkdir -p "$WINDOWS_DEPLOY_DIR" || {
        echo -e "${RED}❌ Failed to create deployment directory: $WINDOWS_DEPLOY_DIR${NC}"
        exit 1
    }
    echo -e "${GREEN}✅ Created directory: $WINDOWS_DEPLOY_DIR${NC}"
else
    echo -e "${GREEN}✅ Directory exists: $WINDOWS_DEPLOY_DIR${NC}"
fi

# Step 5: Copy self-contained published files
PUBLISH_PATH="bin/$BUILD_CONFIG/$TARGET_FRAMEWORK/win-x64/publish"
if [[ -d "$PUBLISH_PATH" ]]; then
    echo -e "${YELLOW}📋 Copying self-contained deployment...${NC}"
    cp -r "$PUBLISH_PATH"/* "$WINDOWS_DEPLOY_DIR/" || {
        echo -e "${RED}❌ Failed to copy published files${NC}"
        exit 1
    }
    echo -e "${GREEN}✅ Copied self-contained deployment from: $PUBLISH_PATH${NC}"
else
    echo -e "${RED}❌ Published files not found: $PUBLISH_PATH${NC}"
    exit 1
fi

# Step 6: Create run script for Windows
echo -e "${YELLOW}📝 Creating Windows run script...${NC}"
cat > "$WINDOWS_DEPLOY_DIR/run-mcp-clipboard.bat" << 'EOF'
@echo off
echo Starting MCP Clipboard Server...
echo Press Ctrl+C to stop
McpClipboard.exe
pause
EOF

echo -e "${GREEN}✅ Created: run-mcp-clipboard.bat${NC}"

# Step 7: Create configuration template
echo -e "${YELLOW}⚙️ Creating Claude Desktop configuration template...${NC}"
cat > "$WINDOWS_DEPLOY_DIR/claude-config-template.json" << EOF
{
  "mcpServers": {
    "mcp-clipboard": {
      "command": "C:\\\\Tools\\\\mcp-clipboard\\\\McpClipboard.exe",
      "args": []
    }
  }
}
EOF

echo -e "${GREEN}✅ Created: claude-config-template.json${NC}"

# Step 8: Create README
echo -e "${YELLOW}📖 Creating README...${NC}"
cat > "$WINDOWS_DEPLOY_DIR/README.md" << 'EOF'
# MCP Clipboard Server (.NET 9)

## Technical Notes
- Built for .NET 9 Windows Desktop
- Uses System.Windows.Forms.Clipboard (not System.Windows.Clipboard)
- Requires STA thread mode for proper COM apartment management
- Handles real-time clipboard change events via Win32 APIs

## Usage

### Option 1: Direct Run
```cmd
run-mcp-clipboard.bat
```

### Option 2: Claude Desktop Integration
1. Copy the contents of `claude-config-template.json`
2. Add to your Claude Desktop configuration file
3. Restart Claude Desktop

## Features
- Real-time clipboard monitoring
- Text and image clipboard support
- File output capabilities
- Chunking for large content

## Configuration File Location
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

## Logs
The application logs to console. For persistent logs, redirect output:
```cmd
McpClipboard.exe > clipboard.log 2>&1
```
EOF

echo -e "${GREEN}✅ Created: README.md${NC}"

# Step 9: Display results
echo ""
echo -e "${GREEN}🎉 Deployment Complete!${NC}"
echo "================================================="
echo -e "${BLUE}📍 Deployed to:${NC} $WINDOWS_DEPLOY_DIR"
echo -e "${BLUE}🔗 Windows path:${NC} C:\\Tools\\mcp-clipboard"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Open Windows Explorer to C:\\Tools\\mcp-clipboard"
echo "2. Run: run-mcp-clipboard.bat (for testing)"
echo "3. Or add to Claude Desktop using claude-config-template.json"
echo ""
echo -e "${GREEN}✅ Ready for use!${NC}"