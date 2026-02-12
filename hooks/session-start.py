#!/usr/bin/env python3
"""SessionStart hook - injects relevant memories at session start.

This hook fires when a Claude Code session begins. It queries the SQLite
memory database for relevant context based on the project name and importance
scores, then injects it into the conversation.

Features:
- Working directory awareness (container tag boosting for current project)
- Importance-based ranking (high-importance memories first)
- Token budget awareness (default: 2000 tokens)
- Fact type prioritization (static > dynamic > session_turn)
- Graceful fallback (hybrid -> regular search -> context)

The hook is non-blocking and fails silently if the memory system is unavailable.
"""

import os
import subprocess
from pathlib import Path
from typing import Optional


def get_memory_binary() -> Optional[str]:
    """Find the mark42 binary."""
    plugin_root = os.environ.get("CLAUDE_PLUGIN_ROOT", "")

    # Try plugin bin directory first
    if plugin_root:
        candidate = Path(plugin_root) / "bin" / "mark42"
        if candidate.exists():
            return str(candidate)

    # Try ~/bin
    candidate = Path.home() / "bin" / "mark42"
    if candidate.exists():
        return str(candidate)

    # Try PATH
    import shutil
    return shutil.which("mark42")


def get_database_path() -> Optional[str]:
    """Get the database path from environment or default."""
    db_path = os.environ.get("CLAUDE_MEMORY_DB", "")
    if not db_path:
        db_path = str(Path.home() / ".claude" / "memory.db")

    return db_path if Path(db_path).exists() else None


def main():
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", os.getcwd())
    project_name = os.path.basename(project_dir)

    memory_bin = get_memory_binary()
    if not memory_bin:
        return

    db_path = get_database_path()
    if not db_path:
        return

    # Configuration from environment
    boost_factor = os.environ.get("CLAUDE_MEMORY_BOOST", "1.5")

    try:
        # Inject recent session summaries first
        recall_result = subprocess.run(
            [
                memory_bin,
                "--db", db_path,
                "session", "recall", project_name,
                "--hours", "72",
                "--tokens", "500",
            ],
            capture_output=True,
            text=True,
            timeout=5,
        )

        if recall_result.returncode == 0 and recall_result.stdout.strip():
            print(recall_result.stdout.strip())
            print()

        # Then, try working directory-aware search with container tag boosting
        result = subprocess.run(
            [
                memory_bin,
                "--db", db_path,
                "workdir", "search", project_name,
                "--tag", project_name,  # Boost memories tagged with this project
                "--boost", boost_factor,
                "--limit", "10",
            ],
            capture_output=True,
            text=True,
            timeout=10,
        )

        if result.returncode == 0 and result.stdout.strip():
            output = result.stdout.strip()
            # Estimate tokens (4 chars ≈ 1 token)
            estimated_tokens = len(output) // 4

            print(f"=== mark42: {project_name} ===")
            print(f"[{estimated_tokens} estimated tokens, boosted for {project_name}]")
            print()
            print(output)
            return

        # Fallback: Try hybrid search without boosting
        result = subprocess.run(
            [
                memory_bin,
                "--db", db_path,
                "hybrid-search", project_name,
                "--limit", "10",
                "--format", "context",
            ],
            capture_output=True,
            text=True,
            timeout=10,
        )

        if result.returncode == 0 and result.stdout.strip():
            output = result.stdout.strip()
            estimated_tokens = len(output) // 4

            print(f"=== mark42: {project_name} ===")
            print(f"[{estimated_tokens} estimated tokens]")
            print()
            print(output)
            return

        # Fallback: Try regular search if hybrid search fails
        result = subprocess.run(
            [
                memory_bin,
                "--db", db_path,
                "search", project_name,
                "--limit", "5",
                "--format", "context",
            ],
            capture_output=True,
            text=True,
            timeout=5,
        )

        if result.returncode == 0 and result.stdout.strip():
            output = result.stdout.strip()
            estimated_tokens = len(output) // 4

            print(f"=== mark42: {project_name} ===")
            print(f"[{estimated_tokens} estimated tokens]")
            print()
            print(output)

    except subprocess.TimeoutExpired:
        # Fail silently - don't block session start
        pass
    except OSError:
        # Binary not found or other OS error
        pass
    except Exception:
        # Any other error - fail silently
        pass


if __name__ == "__main__":
    main()
