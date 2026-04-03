#!/bin/sh
[ -n "$CLAUDE_PLUGIN_ROOT" ] || { echo "[mark42] CLAUDE_PLUGIN_ROOT not set" >&2; exit 1; }
CACHE="${CLAUDE_PLUGIN_ROOT}/.bin-cache/mark42"
[ -x "$CACHE" ] || npx --yes @mfenderov/mark42@latest install-binary "$CACHE" || \
  { echo "[mark42] install-binary failed — hooks disabled until next start" >&2; exit 0; }
exec "$CACHE" "$@"
