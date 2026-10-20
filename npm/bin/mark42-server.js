#!/usr/bin/env node
'use strict';

const path = require('path');
const fs = require('fs');
const { execFileSync } = require('child_process');

const binary = path.join(__dirname, '..', 'bin', 'native', 'mark42-server');

if (!fs.existsSync(binary)) {
  process.stderr.write(`[mark42-server] binary not found — run: npm install @mfenderov/mark42\n`);
  process.exit(1);
}

try {
  execFileSync(binary, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(e.status || 1);
}
