# Temp File Cleanup Implementation Plan - REVISED

## Key Discovery: Files Only Created On-Demand

### Current State Analysis
- ✅ Temp files created with hash-based naming for deduplication
- ✅ Files created in system temp directory (`os.TempDir()`)
- ✅ **IMPORTANT**: Files only created during `read_clipboard` tool calls (when content > 25KB)
- ❌ **NOT created during monitoring** - monitoring only updates `atomic.Value`
- ❌ No cleanup mechanism exists
- ❌ Orphaned files persist after process crashes/reboots

### Simplified Scenarios (Low Frequency)
1. **User requests large clipboard read** - temp file created
2. **Process crash/reboot** - orphaned files persist
3. **Multiple server instances** - safe due to hash-based naming
4. **Accumulated files over time** - only from actual usage, much lower volume

## Recommended Solution: Simple On-Demand + Startup Cleanup

**Strategy**: Much simpler approach since files are only created when needed.

### Core Features:
1. **On-demand cleanup**: Clean old files before creating new ones
2. **Startup cleanup**: Clean orphaned files from previous sessions  
3. **TTL-based aging**: Files older than 1 hour are removed
4. **Optional session tracking**: Track files for graceful cleanup

---

## Simplified Task Implementation Plan

### ⚠️ **LOW RISK**: Infrequent File Operations

**Risks:** File deletion during rare large clipboard operations
**Mitigation:** Safe deletion checks, simple error handling

---

## Task 1: Add On-Demand Cleanup Before File Creation
**Risk**: LOW | **Files**: `/home/beagle/work/mcp-clip/main.go`

**Current State**:
- ✅ Temp files created with hash-based names in `saveToTempFile()`
- ❌ No cleanup before file creation
- ❌ No TTL-based file aging

**Changes**:
- Add timestamp to filename format: `mcp-clip-{timestamp}-{hash}.{ext}`
- Add cleanup call before creating new temp files
- Implement TTL-based cleanup logic

**Success Criteria**:
- [x] New filename format includes timestamp
- [x] `cleanupExpiredFiles()` removes files older than 1 hour
- [x] Cleanup called before each `saveToTempFile()` operation
- [x] Run: `go build && ./mcp-clip test` - no functionality regression
- [x] Test: Create old temp files, trigger read, verify cleanup

**Validation Commands**:
```bash
go build -o mcp-clip
./mcp-clip test
ls /tmp/mcp-clip-* | head -5  # Verify new filename format
```

---

## Task 2: Add Startup Cleanup
**Risk**: LOW | **Files**: `/home/beagle/work/mcp-clip/main.go`

**Current State**:
- ✅ Server startup process exists in `main()`
- ❌ No startup cleanup for orphaned files

**Changes**:
- Add startup cleanup call in `main()` before starting services
- Clean expired mcp-clip files from previous instances

**Success Criteria**:
- [x] Startup cleanup runs once during server initialization
- [x] Only removes files matching mcp-clip pattern and expired TTL
- [x] Run: Create old temp files, restart server, verify cleanup
- [x] Test: Files from other applications are not touched

**Validation Commands**:
```bash
# Create test files with old timestamps
touch /tmp/mcp-clip-$(date -d '2 hours ago' +%s)-test.txt
touch /tmp/other-file.txt
./mcp-clip test
ls /tmp/mcp-clip-* 2>/dev/null | wc -l  # Should be 0
ls /tmp/other-file.txt  # Should still exist
```

---

## Task 3: Add Optional Session File Tracking
**Risk**: LOW | **Files**: `/home/beagle/work/mcp-clip/main.go`

**Current State**:
- ✅ Graceful shutdown with signal handling exists
- ❌ No tracking of files created during session

**Changes**:
- Add simple file tracking to ClipboardServer struct
- Clean tracked files during graceful shutdown

**Success Criteria**:
- [x] Track temp files created during current session
- [x] Clean tracked files during graceful shutdown (SIGTERM/SIGINT)
- [x] Don't interfere with files from other processes
- [x] Run: Create files via tool, send SIGTERM, verify cleanup

**Validation Commands**:
```bash
# Test graceful cleanup
./mcp-clip &
PID=$!
# Trigger large file creation somehow, then:
kill -TERM $PID
wait $PID
# Verify session files cleaned up
```

---

## Task 4: Add Configuration and Documentation  
**Risk**: LOW | **Files**: `/home/beagle/work/mcp-clip/main.go`, `/home/beagle/work/mcp-clip/README.md`

**Current State**:
- ✅ Environment variable support exists (`MCP_DEBUG`)
- ❌ No cleanup configuration
- ✅ README exists but doesn't document temp file behavior

**Changes**:
- Add `MCP_CLEANUP_TTL` environment variable
- Update README with temp file cleanup behavior
- Add debug logging for cleanup operations

**Success Criteria**:
- [x] `MCP_CLEANUP_TTL` controls file age threshold (default: 1 hour)
- [x] README documents temp file creation and cleanup
- [x] Debug logging shows cleanup operations when `MCP_DEBUG=1`
- [x] Run: Test with custom TTL values

**Validation Commands**:
```bash
MCP_CLEANUP_TTL=1800 MCP_DEBUG=1 ./mcp-clip test
# Should show 30-minute TTL in debug output
```

---

## Implementation Strategy

### Simple 3-Phase Approach:

**Phase 1: On-Demand Cleanup** (Task 1)
- Core cleanup logic integrated into existing file creation

**Phase 2: Startup Recovery** (Task 2)  
- Handle orphaned files from crashes/reboots

**Phase 3: Polish & Documentation** (Tasks 3-4)
- Optional session tracking and user documentation

### Configuration Defaults
```go
const (
    DefaultCleanupTTL = 1 * time.Hour  // Files older than 1 hour
    FilenamePrefix    = "mcp-clip-"
)
```

### Filename Format
```
mcp-clip-{unix_timestamp}-{content_hash}.{extension}
Example: mcp-clip-1704312000-a1b2c3d4e5f6.png
```

### Benefits of Simplified Approach:
- ✅ **Much simpler**: No background goroutines or complex state
- ✅ **Low overhead**: Cleanup only when actually needed  
- ✅ **Safe**: TTL-based rules prevent conflicts
- ✅ **Sufficient**: Handles all realistic scenarios for low-frequency file creation

---

## ✅ Implementation Completed Successfully

All tasks have been completed and the temp file cleanup system is fully implemented:

### **Phase 1: On-Demand Cleanup** ✅
- [x] TTL-based file aging with timestamp filename format
- [x] Automatic cleanup before each temp file creation
- [x] Smart cleanup preserves recent files and non-mcp files

### **Phase 2: Startup Recovery** ✅  
- [x] Startup cleanup handles orphaned files from crashes/reboots
- [x] Only removes expired mcp-clip files, preserves other applications

### **Phase 3: Session Tracking & Documentation** ✅
- [x] Optional session file tracking for graceful shutdown cleanup
- [x] Configurable TTL via MCP_CLEANUP_TTL environment variable
- [x] Comprehensive documentation and debug logging

### **Final Implementation Summary:**

🎯 **Simple & Robust**: Clean, maintainable solution without complex background processes
🔒 **Secure**: Restrictive file permissions (0600) and safe cleanup patterns  
⚙️ **Configurable**: Environment variables for different deployment needs
🧹 **Comprehensive**: Handles all failure scenarios (crash, reboot, shutdown, long-running)
📚 **Well-documented**: Complete README with troubleshooting and configuration guides

The temp file cleanup system successfully balances simplicity with robustness, handling all realistic scenarios for the low-frequency file creation pattern of the MCP clipboard server.