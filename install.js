#!/usr/bin/env node

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

const REPO_URL = 'https://github.com/standardbeagle/mcp-clip';
const BINARY_NAME = 'mcp-clip';

function getPlatformInfo() {
  const platform = os.platform();
  const arch = os.arch();
  
  let goos, goarch;
  
  switch (platform) {
    case 'darwin':
      goos = 'darwin';
      break;
    case 'linux':
      goos = 'linux';
      break;
    case 'win32':
      goos = 'windows';
      break;
    default:
      throw new Error(`Unsupported platform: ${platform}`);
  }
  
  switch (arch) {
    case 'x64':
      goarch = 'amd64';
      break;
    case 'arm64':
      goarch = 'arm64';
      break;
    default:
      throw new Error(`Unsupported architecture: ${arch}`);
  }
  
  return { goos, goarch, extension: goos === 'windows' ? '.exe' : '' };
}

function ensureBinDir() {
  const binDir = path.join(__dirname, 'bin');
  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }
  return binDir;
}

async function downloadBinary() {
  const { goos, goarch, extension } = getPlatformInfo();
  const binDir = ensureBinDir();
  const binaryPath = path.join(binDir, BINARY_NAME + extension);
  
  console.log(`Installing mcp-clip for ${goos}/${goarch}...`);
  
  try {
    // Try to download pre-built binary from GitHub releases
    const releaseUrl = `${REPO_URL}/releases/latest/download/mcp-clip-${goos}-${goarch}${extension}`;
    console.log(`Attempting to download: ${releaseUrl}`);
    
    try {
      execSync(`curl -L -o "${binaryPath}" "${releaseUrl}"`, { stdio: 'inherit' });
      
      // Make executable on Unix systems
      if (goos !== 'windows') {
        fs.chmodSync(binaryPath, 0o755);
      }
      
      console.log(`✅ Successfully installed mcp-clip to ${binaryPath}`);
      return;
    } catch (downloadError) {
      console.log('❌ Pre-built binary not available, building from source...');
    }
    
    // Fallback: Build from source using Go
    console.log('Building from source...');
    
    // Check if Go is installed
    try {
      execSync('go version', { stdio: 'ignore' });
    } catch {
      throw new Error('Go is not installed. Please install Go 1.21+ or download a pre-built binary.');
    }
    
    // Build the binary
    const env = {
      ...process.env,
      GOOS: goos,
      GOARCH: goarch,
      CGO_ENABLED: '1'
    };
    
    execSync(`go build -o "${binaryPath}" ${REPO_URL}@latest`, {
      stdio: 'inherit',
      env
    });
    
    // Make executable on Unix systems
    if (goos !== 'windows') {
      fs.chmodSync(binaryPath, 0o755);
    }
    
    console.log(`✅ Successfully built and installed mcp-clip to ${binaryPath}`);
    
  } catch (error) {
    console.error('❌ Installation failed:', error.message);
    console.error('');
    console.error('Manual installation options:');
    console.error('1. Install Go 1.21+ and run: go install github.com/standardbeagle/mcp-clip@latest');
    console.error('2. Download binary from: https://github.com/standardbeagle/mcp-clip/releases');
    console.error('3. Build from source: git clone && cd mcp-clip && go build');
    process.exit(1);
  }
}

// Run installation
downloadBinary().catch(error => {
  console.error('❌ Unexpected error:', error);
  process.exit(1);
});