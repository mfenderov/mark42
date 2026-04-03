#!/usr/bin/env node
'use strict';

const https = require('https');
const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
const os = require('os');
const crypto = require('crypto');

const pkg = require('./package.json');
const version = pkg.version;

if (version === '0.0.0') {
  process.stderr.write('[mark42] Skipping binary download — version is placeholder (dev install)\n');
  process.exit(0);
}
const PLATFORM_MAP = {
  'darwin-arm64': 'darwin_arm64',
  'darwin-x64':   'darwin_amd64',
  'linux-arm64':  'linux_arm64',
  'linux-x64':    'linux_amd64',
};

const key = `${process.platform}-${process.arch}`;
const platform = PLATFORM_MAP[key];
if (!platform) {
  process.stderr.write(`[mark42] Unsupported platform: ${key} — skipping binary install\n`);
  process.exit(0);
}

const nativeDir = path.join(__dirname, 'bin', 'native');
fs.mkdirSync(nativeDir, { recursive: true });

const tarball = `mark42_${version}_${platform}.tar.gz`;
const url = `https://github.com/mfenderov/mark42/releases/download/v${version}/${tarball}`;
const token = process.env.GITHUB_TOKEN || process.env.GH_TOKEN;

process.stderr.write(`[mark42] Downloading mark42 v${version} for ${platform}...\n`);

const tmpFile = path.join(os.tmpdir(), tarball);

function download(url, dest, cb, redirects = 0, sendAuth = true) {
  if (redirects > 5) return cb(new Error('Too many redirects'));
  const headers = { 'User-Agent': 'mark42-installer' };
  if (token && sendAuth) headers['Authorization'] = `Bearer ${token}`;
  const req = https.get(url, { headers }, (res) => {
    if (res.statusCode === 302 || res.statusCode === 301) {
      if (!res.headers.location) return cb(new Error(`Redirect ${res.statusCode} with no Location`));
      res.resume();
      return download(res.headers.location, dest, cb, redirects + 1, false);
    }
    if (res.statusCode !== 200) {
      res.resume();
      return cb(new Error(`HTTP ${res.statusCode} downloading ${url}`));
    }
    res.on('error', cb);
    const file = fs.createWriteStream(dest);
    res.pipe(file);
    file.on('finish', () => file.close((closeErr) => cb(closeErr || null)));
    file.on('error', (writeErr) => {
      file.close(() => {
        try { fs.unlinkSync(dest); } catch (_) {}
        cb(writeErr);
      });
    });
  }).on('error', cb);
  req.setTimeout(30000, () => req.destroy(new Error('Download timed out after 30s')));
}

const checksumsUrl = `https://github.com/mfenderov/mark42/releases/download/v${version}/checksums.txt`;
const tmpChecksums = path.join(os.tmpdir(), `mark42_${version}_checksums.txt`);

download(url, tmpFile, (err) => {
  if (err) {
    try { fs.unlinkSync(tmpFile); } catch (_) {}
    console.error(`[mark42] Download failed: ${err.message}`);
    process.exit(1);
  }

  download(checksumsUrl, tmpChecksums, (checksumErr) => {
    if (checksumErr) {
      try { fs.unlinkSync(tmpFile); } catch (_) {}
      console.error(`[mark42] Failed to download checksums: ${checksumErr.message}`);
      process.exit(1);
    }

    // Verify SHA256
    let checksumContent;
    try {
      checksumContent = fs.readFileSync(tmpChecksums, 'utf8');
    } catch (e) {
      try { fs.unlinkSync(tmpFile); } catch (_) {}
      try { fs.unlinkSync(tmpChecksums); } catch (_) {}
      console.error(`[mark42] Failed to read checksums: ${e.message}`);
      process.exit(1);
    }
    try { fs.unlinkSync(tmpChecksums); } catch (_) {}

    const lines = checksumContent.split('\n');
    const checksumLine = lines.find(l => {
      const parts = l.trim().split(/\s+/);
      return parts.length >= 2 && parts[parts.length - 1] === tarball;
    });

    if (!checksumLine) {
      try { fs.unlinkSync(tmpFile); } catch (_) {}
      console.error(`[mark42] Checksum not found for ${tarball}`);
      process.exit(1);
    }

    const expectedHash = checksumLine.trim().split(/\s+/)[0];
    const actualHash = crypto.createHash('sha256').update(fs.readFileSync(tmpFile)).digest('hex');

    if (actualHash !== expectedHash) {
      try { fs.unlinkSync(tmpFile); } catch (_) {}
      console.error(`[mark42] SHA256 mismatch for ${tarball}: expected ${expectedHash}, got ${actualHash}`);
      process.exit(1);
    }

    // Extraction (Block 1)
    try {
      execFileSync('tar', ['xzf', tmpFile, '-C', nativeDir, 'mark42', 'mark42-server'], { stdio: 'inherit' });
    } catch (e) {
      try { fs.unlinkSync(tmpFile); } catch (_) {}
      console.error(`[mark42] Extraction failed: ${e.message}`);
      process.exit(1);
    }
    fs.unlinkSync(tmpFile);

    // Chmod (Block 2)
    try {
      fs.chmodSync(path.join(nativeDir, 'mark42'), 0o755);
      fs.chmodSync(path.join(nativeDir, 'mark42-server'), 0o755);
    } catch (e) {
      console.error(`[mark42] Failed to set binary permissions: ${e.message}`);
      process.exit(1);
    }
    process.stderr.write('[mark42] ✓ Done\n');
  }, 0, true);
});
