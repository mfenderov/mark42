#!/usr/bin/env node
'use strict';

const path = require('path');
const fs = require('fs');
const { execFileSync } = require('child_process');

const binary = path.join(__dirname, '..', 'bin', 'native', 'mark42');

if (!fs.existsSync(binary) && process.argv[2] !== 'install-binary') {
  process.stderr.write(`[mark42] binary not found — run: npm install @mfenderov/mark42\n`);
  process.exit(1);
}

if (process.argv[2] === 'install-binary') {
  const dest = process.argv[3];
  if (!dest) {
    console.error('Usage: install-binary <path>');
    process.exit(1);
  }
  try {
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(binary, dest);
    fs.chmodSync(dest, 0o755);
  } catch (e) {
    process.stderr.write(`[mark42] install-binary failed: ${e.message}\n`);
    process.exit(1);
  }
  process.exit(0);
}

try {
  execFileSync(binary, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(e.status || 1);
}
