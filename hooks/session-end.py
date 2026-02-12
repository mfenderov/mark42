#!/usr/bin/env python3
"""SessionEnd hook - final cleanup and session statistics."""

import json
import os
import sys
from pathlib import Path


def main():
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if not project_dir:
        return

    # Read session data from stdin
    try:
        session_data = json.loads(sys.stdin.read())
    except (json.JSONDecodeError, Exception):
        session_data = {}

    stats = {
        "project": Path(project_dir).name,
    }

    # Check for dirty files that weren't synced
    dirty_file = Path(project_dir) / ".claude" / "mark42" / "dirty-files"
    if dirty_file.exists() and dirty_file.stat().st_size > 0:
        files = [f.strip() for f in dirty_file.read_text().split("\n") if f.strip()]
        stats["unsynced_files"] = len(files)

    # Add session stats if available
    if "transcript_length" in session_data:
        stats["transcript_chars"] = session_data["transcript_length"]

    # SessionEnd hooks must not output structured JSON
    # (hookSpecificOutput is only valid for PreToolUse/PermissionRequest)
    # Stats are collected silently for future use


if __name__ == "__main__":
    main()
