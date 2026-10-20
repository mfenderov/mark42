# mark42 Distribution Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace brew + manual MCP setup with `claude plugin install mark42@mark42` as the only install command.

**Architecture:** An npm package `@mfenderov/mark42` wraps the existing Go binaries — no Go code changes. The plugin ships `.mcp.json` for auto MCP registration and `hook.sh` for fast cached hook execution. Goreleaser publishes the Go binaries to GitHub releases; CI publishes the npm wrapper.

**Tech Stack:** Node.js ≥18 (install.js, launchers), POSIX sh (hook.sh), goreleaser v2, GitHub Actions, npm

---

## File Map

**New files:**
- `npm/package.json` — npm package definition
- `npm/install.js` — postinstall: downloads Go binaries from GitHub releases
- `npm/bin/mark42.js` — CLI launcher + `install-binary` subcommand
- `npm/bin/mark42-server.js` — MCP server launcher
- `npm/.npmignore` — excludes `bin/native/` from published package
- `plugin/mark42/.mcp.json` — auto-registers MCP on plugin install
- `plugin/mark42/bin/hook.sh` — cached binary launcher

**Modified files:**
- `plugin/mark42/hooks/hooks.json` — hook commands route through `hook.sh`
- `.goreleaser.yml` — remove `brews:`, add jq version sync
- `.github/workflows/release.yml` — add npm publish step, swap secrets
- `Makefile` — remove homebrew copy line
- `.gitignore` — add `npm/bin/native/`
- `README.md` — update install instructions

---

### Task 1: npm package skeleton

**Files:**
- Create: `npm/package.json`
- Create: `npm/.npmignore`
- Modify: `.gitignore`

- [ ] **Step 1: Create `npm/package.json`**

```json
{
  "name": "@mfenderov/mark42",
  "version": "0.0.0",
  "description": "Local, privacy-first RAG memory system for Claude Code",
  "bin": {
    "mark42": "./bin/mark42.js",
    "mark42-server": "./bin/mark42-server.js"
  },
  "scripts": {
    "postinstall": "node install.js",
    "prepublishOnly": "node -e \"const v=require('./package.json').version; if(!v.match(/^\\d/))throw new Error('version not set: '+v)\""
  },
  "files": ["bin/", "install.js"],
  "engines": { "node": ">=18" },
  "license": "MIT",
  "homepage": "https://github.com/mfenderov/mark42"
}
```

- [ ] **Step 2: Create `npm/.npmignore`**

```
bin/native/
```

- [ ] **Step 3: Add to root `.gitignore`**

Append `npm/bin/native/` to `.gitignore`.

- [ ] **Step 4: Create the `npm/bin/` directory**

```bash
mkdir -p npm/bin
```

- [ ] **Step 5: Verify**

```bash
cat npm/package.json
cat npm/.npmignore
grep 'npm/bin/native' .gitignore
```

Expected: all three files present and correct.

- [ ] **Step 6: Commit**

```bash
git add npm/package.json npm/.npmignore .gitignore
git commit -m "chore: add npm package skeleton for binary distribution"
```

---

### Task 2: Binary downloader (`npm/install.js`)

**Files:**
- Create: `npm/install.js`

Runs as `postinstall` hook. Downloads the goreleaser tarball for the current platform, extracts both `mark42` and `mark42-server` into `npm/bin/native/`.

Goreleaser artifact naming (from `.goreleaser.yml` `name_template`):
`mark42_{version}_{os}_{arch}.tar.gz`

Each tarball contains both binaries (goreleaser bundles all builds per OS/arch).

- [ ] **Step 1: Create `npm/install.js`**

```js
#!/usr/bin/env node
'use strict';

const https = require('https');
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');
const os = require('os');

const pkg = require('./package.json');
const version = pkg.version;

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

function download(url, dest, cb) {
  const headers = { 'User-Agent': 'mark42-installer' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  https.get(url, { headers }, (res) => {
    if (res.statusCode === 302 || res.statusCode === 301) {
      return download(res.headers.location, dest, cb);
    }
    if (res.statusCode !== 200) {
      return cb(new Error(`HTTP ${res.statusCode} downloading ${url}`));
    }
    const file = fs.createWriteStream(dest);
    res.pipe(file);
    file.on('finish', () => file.close(cb));
    file.on('error', cb);
  }).on('error', cb);
}

download(url, tmpFile, (err) => {
  if (err) {
    console.error(`[mark42] Download failed: ${err.message}`);
    process.exit(1);
  }
  try {
    execSync(
      `tar xzf "${tmpFile}" -C "${nativeDir}" mark42 mark42-server`,
      { stdio: 'inherit' }
    );
    fs.chmodSync(path.join(nativeDir, 'mark42'), 0o755);
    fs.chmodSync(path.join(nativeDir, 'mark42-server'), 0o755);
    fs.unlinkSync(tmpFile);
    process.stderr.write(`[mark42] ✓ Done\n`);
  } catch (e) {
    console.error(`[mark42] Extraction failed: ${e.message}`);
    process.exit(1);
  }
});
```

- [ ] **Step 2: Test locally against a real release**

Requires v3.0.2 (or whatever the current release is) to exist on GitHub.

```bash
cd npm
node install.js
# Expected:
# [mark42] Downloading mark42 v3.0.2 for darwin_arm64...
# [mark42] ✓ Done
ls bin/native/
# Expected: mark42  mark42-server
./bin/native/mark42 version
# Expected: 3.0.2
cd ..
```

- [ ] **Step 3: Commit**

```bash
git add npm/install.js
git commit -m "chore: add install.js to download Go binaries on npm install"
```

---

### Task 3: npm bin launchers

**Files:**
- Create: `npm/bin/mark42.js`
- Create: `npm/bin/mark42-server.js`

`mark42.js` additionally handles the `install-binary <path>` subcommand used by `hook.sh` to prime the local cache.

- [ ] **Step 1: Create `npm/bin/mark42.js`**

```js
#!/usr/bin/env node
'use strict';

const path = require('path');
const fs = require('fs');
const { execFileSync } = require('child_process');

const binary = path.join(__dirname, '..', 'bin', 'native', 'mark42');

if (process.argv[2] === 'install-binary') {
  const dest = process.argv[3];
  if (!dest) {
    console.error('Usage: install-binary <path>');
    process.exit(1);
  }
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.copyFileSync(binary, dest);
  fs.chmodSync(dest, 0o755);
  process.exit(0);
}

try {
  execFileSync(binary, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(e.status || 1);
}
```

- [ ] **Step 2: Create `npm/bin/mark42-server.js`**

```js
#!/usr/bin/env node
'use strict';

const path = require('path');
const { execFileSync } = require('child_process');

const binary = path.join(__dirname, '..', 'bin', 'native', 'mark42-server');

try {
  execFileSync(binary, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(e.status || 1);
}
```

- [ ] **Step 3: Make launchers executable**

```bash
chmod +x npm/bin/mark42.js npm/bin/mark42-server.js
```

- [ ] **Step 4: Test launchers (requires Task 2 to have run)**

```bash
cd npm
node bin/mark42.js version
# Expected: mark42 3.0.2 (or current version)

node bin/mark42.js install-binary /tmp/mark42-cached
/tmp/mark42-cached version
# Expected: same version
rm /tmp/mark42-cached
cd ..
```

- [ ] **Step 5: Commit**

```bash
git add npm/bin/mark42.js npm/bin/mark42-server.js
git commit -m "chore: add npm bin launchers with install-binary subcommand"
```

---

### Task 4: Plugin — `.mcp.json`

**Files:**
- Create: `plugin/mark42/.mcp.json`

Claude Code reads this on plugin install and auto-registers the MCP server. No manual `claude mcp add` needed.

- [ ] **Step 1: Create `plugin/mark42/.mcp.json`**

```json
{
  "mcpServers": {
    "mark42": {
      "command": "npx",
      "args": ["--yes", "@mfenderov/mark42@latest"],
      "timeout": 120000,
      "env": { "CLAUDE_MEMORY_DB": "${HOME}/.claude/memory.db" }
    }
  }
}
```

- [ ] **Step 2: Verify**

```bash
ls plugin/mark42/
# Expected: agents  commands  hooks  skills  .mcp.json
cat plugin/mark42/.mcp.json
```

- [ ] **Step 3: Commit**

```bash
git add plugin/mark42/.mcp.json
git commit -m "feat: add .mcp.json to auto-register MCP server on plugin install"
```

---

### Task 5: Plugin — `hook.sh`

**Files:**
- Create: `plugin/mark42/bin/hook.sh`

First invocation: calls `npx install-binary` to prime the cache. All subsequent invocations: calls the cached binary directly — no Node.js overhead.

Cache is invalidated by the user running `claude plugin update mark42@mark42` and then manually deleting the cache (`rm ~/.claude/.../bin-cache/mark42`), or on plugin reinstall. The MCP server always auto-updates via `@latest` regardless.

- [ ] **Step 1: Create `plugin/mark42/bin/` directory**

```bash
mkdir -p plugin/mark42/bin
```

- [ ] **Step 2: Create `plugin/mark42/bin/hook.sh`**

```sh
#!/bin/sh
CACHE="${CLAUDE_PLUGIN_ROOT}/.bin-cache/mark42"
[ -x "$CACHE" ] || npx --yes @mfenderov/mark42@latest install-binary "$CACHE"
exec "$CACHE" "$@"
```

- [ ] **Step 3: Test hook.sh locally**

```bash
export CLAUDE_PLUGIN_ROOT=/tmp/mark42-plugin-test
mkdir -p "$CLAUDE_PLUGIN_ROOT"
cp plugin/mark42/bin/hook.sh "$CLAUDE_PLUGIN_ROOT/hook.sh"

sh "$CLAUDE_PLUGIN_ROOT/hook.sh" version
# Expected: downloads binary on first run, prints mark42 version

sh "$CLAUDE_PLUGIN_ROOT/hook.sh" version
# Expected: instant — uses cached binary

ls "$CLAUDE_PLUGIN_ROOT/.bin-cache/"
# Expected: mark42

rm -rf /tmp/mark42-plugin-test
```

- [ ] **Step 4: Commit**

```bash
git add plugin/mark42/bin/hook.sh
git commit -m "feat: add hook.sh with cached binary for fast hook invocations"
```

---

### Task 6: Plugin — update `hooks/hooks.json`

**Files:**
- Modify: `plugin/mark42/hooks/hooks.json`

Switch all four hook commands to route through `hook.sh`. Uses `sh hook.sh` (not `./hook.sh`) so no executable bit required after git clone.

- [ ] **Step 1: Read current `plugin/mark42/hooks/hooks.json`**

```bash
cat plugin/mark42/hooks/hooks.json
```

- [ ] **Step 2: Replace `plugin/mark42/hooks/hooks.json`**

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook session-start",
            "timeout": 10
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write|Bash",
        "hooks": [
          {
            "type": "command",
            "command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook post-tool-use",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook stop",
            "timeout": 30
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "manual|auto",
        "hooks": [
          {
            "type": "command",
            "command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook pre-compact",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

- [ ] **Step 3: Commit**

```bash
git add plugin/mark42/hooks/hooks.json
git commit -m "feat: route all hooks through hook.sh for cached binary execution"
```

---

### Task 7: Release pipeline — goreleaser

**Files:**
- Modify: `.goreleaser.yml`

Two changes: add jq version sync hook, remove the `brews:` block.

- [ ] **Step 1: Read current `.goreleaser.yml`**

```bash
cat .goreleaser.yml
```

- [ ] **Step 2: Add `before.hooks` at the top of `.goreleaser.yml`** (before `builds:`)

```yaml
before:
  hooks:
    - sh -c 'jq --arg v "{{ .Version }}" ".version = $v" npm/package.json > npm/package.json.tmp && mv npm/package.json.tmp npm/package.json'
```

- [ ] **Step 3: Remove the entire `brews:` block from `.goreleaser.yml`**

Delete from `brews:` down to (and including) the final line of the brews block, which ends before `checksum:`.

- [ ] **Step 4: Validate goreleaser config**

```bash
goreleaser check
# Expected: no errors
```

- [ ] **Step 5: Verify jq command works**

```bash
jq --arg v "9.9.9" '.version = $v' npm/package.json
# Expected: prints package.json with "version": "9.9.9"
# (does NOT modify the file — this is just a dry-run check)
```

- [ ] **Step 6: Commit**

```bash
git add .goreleaser.yml
git commit -m "chore: remove brew tap, add npm version sync to goreleaser"
```

---

### Task 8: Release pipeline — GitHub Actions

**Files:**
- Modify: `.github/workflows/release.yml`

Add npm publish after goreleaser. Remove `HOMEBREW_TAP_GITHUB_TOKEN`.

- [ ] **Step 1: Read current `.github/workflows/release.yml`**

```bash
cat .github/workflows/release.yml
```

- [ ] **Step 2: Remove `HOMEBREW_TAP_GITHUB_TOKEN` from the goreleaser step env**

Delete the line:
```yaml
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

- [ ] **Step 3: Add npm publish steps after the goreleaser step**

```yaml
      - uses: actions/setup-node@v4
        with:
          node-version: '24'
          registry-url: 'https://registry.npmjs.org'

      - name: Publish to npm
        run: npm publish --access public
        working-directory: npm
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "chore: add npm publish to release workflow, remove brew tap secret"
```

- [ ] **Step 5: Add `NPM_TOKEN` secret in GitHub**

Go to https://github.com/mfenderov/mark42/settings/secrets/actions

Add: `NPM_TOKEN` — npm access token with publish rights to `@mfenderov` scope.
Remove: `HOMEBREW_TAP_TOKEN`

---

### Task 9: Cleanup + README

**Files:**
- Modify: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Remove homebrew copy from `Makefile`**

In the `install-all` target, remove this line:
```makefile
	cp $(BINARY) $(SERVER) /opt/homebrew/bin/ 2>/dev/null || true
```

- [ ] **Step 2: Update install section in `README.md`**

Replace the installation section with:

```markdown
## Installation

```bash
claude plugin install mark42@mark42
```

That's it. The MCP server registers automatically. The binary downloads on first Claude Code start (~30s one-time). All subsequent starts are instant.

### Updating

```bash
claude plugin update mark42@mark42
```

### Migration from brew

If you previously installed via brew:

```bash
claude mcp remove mark42 --scope user
claude plugin install mark42@mark42
```
```

- [ ] **Step 3: Commit**

```bash
git add Makefile README.md
git commit -m "docs: update install instructions for plugin-only distribution"
```

---

### Task 10: End-to-end verification

- [ ] **Step 1: Dry-run npm publish**

```bash
cd npm
npm publish --dry-run --access public
# Expected output includes:
#   npm notice === Tarball Contents ===
#   npm notice 40B  .npmignore
#   npm notice ...  bin/mark42.js
#   npm notice ...  bin/mark42-server.js
#   npm notice ...  install.js
#   npm notice ...  package.json
# Should NOT include: bin/native/
cd ..
```

- [ ] **Step 2: Verify goreleaser snapshot build**

```bash
goreleaser build --snapshot --clean
# Expected: builds 4 platform binaries successfully, no errors
cat npm/package.json | grep version
# Expected: "version": "0.0.0" (snapshot doesn't run before.hooks)
```

- [ ] **Step 3: Full end-to-end test (manual, after first real release)**

1. Tag and push: `git tag v<next> && git push origin v<next>`
2. Wait for CI to complete (goreleaser + npm publish)
3. `claude plugin install mark42@mark42`
4. Start Claude Code
5. Verify MCP connects in Claude Code's MCP panel
6. Edit a file — verify `post-tool-use` hook fires (check session DB: `mark42 stats`)
7. End session — verify stop hook fires (session captured in DB)

- [ ] **Step 4: Migration self-test**

```bash
claude mcp remove mark42 --scope user
# Verify old entry gone:
claude mcp list
# Should not show mark42 under user scope
```
