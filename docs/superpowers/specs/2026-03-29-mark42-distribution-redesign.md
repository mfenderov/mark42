# mark42 Distribution Redesign

**Date:** 2026-03-29
**Status:** Approved

## Goal

Replace brew + manual MCP setup with a single install command:

```bash
claude plugin install mark42@mark42
```

Binary distributed via npm (`@mfenderov/mark42`), auto-updated transparently via `@latest`. No brew, no `claude mcp add`, no manual steps.

## Architecture

```
GitHub Release (goreleaser)
  └── mark42_VERSION_OS_ARCH.tar.gz  (unchanged)

npm registry
  └── @mfenderov/mark42@latest
        ├── package.json            (bins: mark42, mark42-server)
        ├── install.js              (postinstall: downloads Go binaries)
        └── bin/
            ├── mark42.js           (launcher → bin/native/mark42)
            └── mark42-server.js    (launcher → bin/native/mark42-server)

Claude Code plugin (mfenderov/mark42 repo, plugin/mark42/)
  ├── .mcp.json                     (auto-registers MCP on plugin install)
  ├── bin/hook.sh                   (caches binary path, calls directly)
  ├── hooks/hooks.json              (sh hook.sh — no executability assumption)
  ├── agents/
  ├── skills/
  └── commands/
```

## Data Flow

**First install:**
1. `claude plugin install mark42@mark42` — plugin cloned, `.mcp.json` registered
2. First Claude Code start — `npx @mfenderov/mark42@latest` downloads npm package,
   `install.js` downloads both Go binaries (~11MB each), MCP server starts (timeout: 120s)
3. First hook fires — `hook.sh` sees no cache, runs `install-binary`, caches binary, calls it
4. All subsequent starts — npm cache hit, binary path cached, everything instant

**Updates:**
1. New git tag → goreleaser → new GitHub release + npm package published
2. `claude plugin update mark42@mark42` — pulls new `hook.sh` (cache deleted, re-downloads)
3. Next Claude Code start — MCP server gets new binary via npx; hook re-caches

## Components

### npm package: `npm/`

**`npm/package.json`**
```json
{
  "name": "@mfenderov/mark42",
  "version": "0.0.0",
  "bin": {
    "mark42": "./bin/mark42.js",
    "mark42-server": "./bin/mark42-server.js"
  },
  "scripts": {
    "postinstall": "node install.js",
    "prepublishOnly": "node -e \"const v=require('./package.json').version; if(!v.match(/^\\d/))throw new Error('version not set: '+v)\""
  },
  "files": ["bin/", "install.js"],
  "engines": { "node": ">=18" }
}
```

**`npm/.npmignore`**
```
bin/native/
```

**`npm/install.js`** (~40 lines, zero external dependencies)
- Maps `process.platform + process.arch` to goreleaser artifact names
  - `.tar.gz` for darwin/linux, `.zip` for windows
- Prints progress to stderr: `Downloading mark42 vX.Y.Z...` / `✓ Done`
- Downloads tarball from GitHub releases for the current package version
- Extracts both `mark42` and `mark42-server` into `bin/native/`
- Sets executable bit (`chmod 0o755`)
- Supports `GITHUB_TOKEN` env var for authenticated requests

**`npm/bin/mark42.js` and `npm/bin/mark42-server.js`** (~5 lines each)
- Resolve path to native binary in `bin/native/`
- `execFileSync` with `process.argv.slice(2)` and `{ stdio: 'inherit' }`

**`npm/bin/mark42.js` also supports `install-binary <path>` subcommand:**
- Copies the resolved native binary to `<path>`

### Plugin changes: `plugin/mark42/`

**New: `.mcp.json`**
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

**New: `bin/hook.sh`**
```sh
#!/bin/sh
CACHE="${CLAUDE_PLUGIN_ROOT}/.bin-cache/mark42"
[ -x "$CACHE" ] || npx --yes @mfenderov/mark42@latest install-binary "$CACHE"
exec "$CACHE" "$@"
```
First invocation: downloads and caches the binary. All subsequent: calls binary directly.
Cache cleared by new `hook.sh` on plugin update → triggers fresh download.

**Updated: `hooks/hooks.json`**
```json
"command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook session-start"
"command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook post-tool-use"
"command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook stop"
"command": "sh ${CLAUDE_PLUGIN_ROOT}/bin/hook.sh hook pre-compact"
```

### Release pipeline changes

**`.goreleaser.yml`**
- Remove entire `brews:` block
- Add version sync in `before.hooks`:
  ```yaml
  before:
    hooks:
      - sh -c 'jq --arg v "{{ .Version }}" ".version = $v" npm/package.json > npm/package.json.tmp && mv npm/package.json.tmp npm/package.json'
  ```
- Ensure `release.draft: false`

**`.github/workflows/release.yml`**
Add after goreleaser step:
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

Secrets: add `NPM_TOKEN`, remove `HOMEBREW_TAP_TOKEN`.

**`Makefile`**
- Remove `/opt/homebrew/bin/` copy line from `install-all` target

### Root `.gitignore`
Add: `npm/bin/native/`

## Migration (existing users)

1. `claude mcp remove mark42 --scope user` — removes manual MCP registration
2. `claude plugin install mark42@mark42` — installs updated plugin, auto-registers MCP
3. First Claude Code start — binary downloads automatically (~30s first time only)

The brew-installed binary can remain; no longer used by the plugin.

## Files Changed

| File | Change |
|------|--------|
| `npm/package.json` | New |
| `npm/install.js` | New |
| `npm/bin/mark42.js` | New |
| `npm/bin/mark42-server.js` | New |
| `npm/.npmignore` | New |
| `plugin/mark42/.mcp.json` | New |
| `plugin/mark42/bin/hook.sh` | New |
| `plugin/mark42/hooks/hooks.json` | Update — `sh hook.sh` |
| `.goreleaser.yml` | Remove `brews:`, add jq version sync |
| `.github/workflows/release.yml` | Add npm publish step, swap secrets |
| `Makefile` | Remove homebrew copy line |
| `.gitignore` | Add `npm/bin/native/` |
| `README.md` | Update install instructions |

## What Does Not Change

- Go source code
- Goreleaser binary builds (all 4 platforms)
- GitHub release artifacts
- Plugin assets: agents, skills, commands
- Database schema and MCP tools
