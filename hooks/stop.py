#!/usr/bin/env python3
"""Stop hook - terse memory sync with context summary."""

import json
import os
import sys
from pathlib import Path


def main():
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if not project_dir:
        return

    # Read session input from Claude Code
    try:
        session_data = json.loads(sys.stdin.read())
    except (json.JSONDecodeError, Exception):
        session_data = {}

    # Gather context to help Claude
    context_parts = []

    # 1. Project name
    project_name = Path(project_dir).name
    context_parts.append(f"Project: {project_name}")

    # 2. Dirty files (what was modified)
    files = []
    dirty_file = Path(project_dir) / ".claude" / "claude-memory" / "dirty-files"
    if dirty_file.exists() and dirty_file.stat().st_size > 0:
        files = [f.strip() for f in dirty_file.read_text().split("\n") if f.strip()][:5]
        if files:
            context_parts.append(f"Files modified: {', '.join(files)}")

    # 3. Session stats if available
    if "transcript_length" in session_data:
        context_parts.append(f"Session length: {session_data['transcript_length']} chars")

    files_list = ", ".join(files) if files else "none"

    output = {
        "decision": "block",
        "reason": (
            f"Memory sync for {project_name}. "
            f"Modified: {files_list}. "
            f"Use create_or_update_entities for new learnings, add_observations for updates. "
            f"Use fact_type='static' for conventions/patterns, 'dynamic' for decisions/context. "
            f"Reply only: 'Synced N learnings.'"
        ),
        "suppressOutput": True,
    }
    print(json.dumps(output))


if __name__ == "__main__":
    main()
