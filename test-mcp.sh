#!/bin/bash

# Test MCP clipboard server via WSL stdio bridge
cd /mnt/c/tmp/McpClipboard/bin/Debug/net9.0-windows

# Function to send MCP request and capture response
send_mcp_request() {
    local request="$1"
    echo "$request" | powershell.exe -Command "dotnet McpClipboard.dll" 2>&1
}

echo "=== Testing MCP Clipboard Server via WSL stdio bridge ==="

echo "1. Testing initialize request..."
INIT_REQUEST='{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"test-client","version":"1.0.0"},"capabilities":{"roots":{"listChanged":true},"sampling":{}}},"id":1}'
send_mcp_request "$INIT_REQUEST"

echo -e "\n2. Testing tools/list request..."
TOOLS_REQUEST='{"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}'
send_mcp_request "$TOOLS_REQUEST"

echo -e "\n3. Done."