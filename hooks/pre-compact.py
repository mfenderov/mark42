#!/usr/bin/env python3
"""PreCompact hook - preserve critical context before compaction."""

import json
import os
import sys
from pathlib import Path


def main():
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if not project_dir:
        return

    # Read compaction data from stdin
    try:
        compact_data = json.loads(sys.stdin.read())
    except (json.JSONDecodeError, Exception):
        compact_data = {}

    project_name = Path(project_dir).name
    preserved_count = 0

    # Check for important context to preserve
    dirty_file = Path(project_dir) / ".claude" / "claude-memory" / "dirty-files"
    if dirty_file.exists() and dirty_file.stat().st_size > 0:
        files = [f.strip() for f in dirty_file.read_text().split("\n") if f.strip()]
        preserved_count = len(files)

    # Output hookSpecificOutput for PreCompact
    output = {
        "hookSpecificOutput": {
            "memoriesPreserved": preserved_count,
            "project": project_name,
            "message": f"Pre-compaction: {preserved_count} files tracked for {project_name}"
        }
    }
    print(json.dumps(output))


if __name__ == "__main__":
    main()
