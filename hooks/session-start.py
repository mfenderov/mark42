#!/usr/bin/env python3
"""SessionStart hook - injects relevant memories at session start.

This hook fires when a Claude Code session begins. It queries the SQLite
memory database for relevant context based on the project name and injects
it into the conversation.

The hook is non-blocking and fails silently if the memory system is unavailable.
"""

import os
import subprocess
import sys
from pathlib import Path


def main():
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", os.getcwd())
    plugin_root = os.environ.get("CLAUDE_PLUGIN_ROOT", "")

    # Find claude-memory binary
    memory_bin = None
    if plugin_root:
        candidate = Path(plugin_root) / "bin" / "claude-memory"
        if candidate.exists():
            memory_bin = str(candidate)

    if not memory_bin:
        # Try ~/bin
        candidate = Path.home() / "bin" / "claude-memory"
        if candidate.exists():
            memory_bin = str(candidate)

    if not memory_bin:
        return

    # Check if database exists
    db_path = os.environ.get("CLAUDE_MEMORY_DB", "")
    if not db_path:
        db_path = str(Path.home() / ".claude" / "memory.db")

    if not Path(db_path).exists():
        return

    # Search for project-relevant memories
    project_name = os.path.basename(project_dir)

    try:
        result = subprocess.run(
            [memory_bin, "--db", db_path, "search", project_name, "--limit", "5", "--format", "context"],
            capture_output=True,
            text=True,
            timeout=5,
        )

        if result.returncode == 0 and result.stdout.strip():
            print(f"Relevant memories for '{project_name}':")
            print()
            print(result.stdout)

    except (subprocess.TimeoutExpired, OSError):
        # Fail silently - don't block session start
        pass


if __name__ == "__main__":
    main()
