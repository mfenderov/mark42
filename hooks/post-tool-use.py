#!/usr/bin/env python3
"""PostToolUse hook - tracks edited files for memory updates.

Fires after Edit, Write, or Bash tool execution. Appends changed file
paths to .claude/mark42/dirty-files for batch processing at turn end.
Produces NO OUTPUT to maintain zero token cost (critical for performance).

Supports configurable trigger modes:
- default: Track Edit/Write/Bash operations (current behavior)
- gitmode: Only track git commits
"""

from __future__ import annotations

import json
import os
import shlex
import subprocess
import sys
from pathlib import Path


def load_config(project_dir: str) -> dict:
    """Load plugin configuration from .claude/mark42/config.json."""
    config_file = Path(project_dir) / ".claude" / "mark42" / "config.json"
    if config_file.exists():
        try:
            with open(config_file) as f:
                return json.load(f)
        except (json.JSONDecodeError, OSError):
            pass
    return {"triggerMode": "default"}


def handle_git_commit(project_dir: str) -> tuple[list[str], dict | None]:
    """Extract context from a git commit.

    Returns: (files, commit_context) where commit_context is {"hash": ..., "message": ...}
    """
    result = subprocess.run(
        ["git", "log", "-1", "--format=%h %s"],
        capture_output=True,
        text=True,
        cwd=project_dir,
    )
    if result.returncode != 0:
        return [], None

    parts = result.stdout.strip().split(" ", 1)
    commit_hash = parts[0]
    commit_message = parts[1] if len(parts) > 1 else ""

    result = subprocess.run(
        ["git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"],
        capture_output=True,
        text=True,
        cwd=project_dir,
    )
    if result.returncode != 0:
        return [], {"hash": commit_hash, "message": commit_message}

    files = [f.strip() for f in result.stdout.strip().split("\n") if f.strip()]
    files = [str((Path(project_dir) / f).resolve()) for f in files]

    return files, {"hash": commit_hash, "message": commit_message}


def should_track(file_path: str, project_dir: str) -> bool:
    """Check if file should be tracked for memory updates."""
    path = Path(file_path)

    try:
        relative = path.relative_to(project_dir)
    except ValueError:
        return False

    # Exclude .claude/ directory (plugin state files)
    if relative.parts and relative.parts[0] == ".claude":
        return False

    # Exclude CLAUDE.md files anywhere
    if path.name == "CLAUDE.md":
        return False

    return True


def extract_files_from_bash(command: str, project_dir: str) -> list[str]:
    """Extract file paths from Bash commands that modify files."""
    if not command:
        return []

    command = command.strip()

    skip_prefixes = (
        "ls", "cat", "echo", "grep", "find", "head", "tail", "less", "more",
        "cd", "pwd", "which", "whereis", "type", "file", "stat", "wc",
        "git status", "git log", "git diff", "git show", "git branch",
        "git fetch", "git pull", "git push", "git clone", "git checkout",
        "git stash", "git remote", "git tag", "git rev-parse",
        "npm ", "yarn ", "pnpm ", "node ", "python", "pip ", "uv ",
        "cargo ", "go ", "make", "cmake", "docker ", "kubectl ",
        "curl ", "wget ", "ssh ", "scp ", "rsync ",
    )
    if command.startswith(skip_prefixes):
        return []

    shell_operators = ("&&", "||", ";", "|", ">", ">>", "<", "2>", "2>&1")
    files = []

    try:
        tokens = shlex.split(command)
        if not tokens:
            return []

        cmd = tokens[0]

        if cmd == "rm":
            for token in tokens[1:]:
                if token in shell_operators:
                    break
                if not token.startswith("-"):
                    files.append(token)

        elif cmd == "git" and len(tokens) > 1 and tokens[1] == "rm":
            for token in tokens[2:]:
                if token in shell_operators:
                    break
                if not token.startswith("-"):
                    files.append(token)

        elif cmd == "mv" and len(tokens) >= 3:
            for token in tokens[1:]:
                if token in shell_operators:
                    break
                if not token.startswith("-"):
                    files.append(token)
                    break

        elif cmd == "git" and len(tokens) > 2 and tokens[1] == "mv":
            for token in tokens[2:]:
                if token in shell_operators:
                    break
                if not token.startswith("-"):
                    files.append(token)
                    break

        elif cmd == "unlink" and len(tokens) > 1:
            if tokens[1] not in shell_operators:
                files.append(tokens[1])

    except ValueError:
        return []

    resolved = []
    for f in files:
        path = Path(f)
        if not path.is_absolute():
            path = Path(project_dir) / path
        resolved.append(str(path.resolve()))

    return resolved


def main():
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if not project_dir:
        return

    try:
        stdin_data = sys.stdin.read()
        tool_input = json.loads(stdin_data) if stdin_data else {}
    except json.JSONDecodeError:
        tool_input = {}

    tool_name = tool_input.get("tool_name", "")
    tool_input_data = tool_input.get("tool_input", {})

    config = load_config(project_dir)
    trigger_mode = config.get("triggerMode", "default")

    is_git_commit = False
    command = ""
    if tool_name == "Bash":
        command = tool_input_data.get("command", "").strip()
        is_git_commit = "git commit" in command

    if trigger_mode == "gitmode" and not is_git_commit:
        return

    files_to_track = []
    commit_context = None

    if is_git_commit:
        files, commit_context = handle_git_commit(project_dir)
        files_to_track.extend(files)
    elif tool_name in ("Edit", "Write"):
        file_path = tool_input_data.get("file_path", "")
        if file_path:
            files_to_track.append(file_path)
    elif tool_name == "Bash":
        files_to_track = extract_files_from_bash(command, project_dir)
    elif not tool_name:
        file_path = tool_input_data.get("file_path", "")
        if file_path:
            files_to_track.append(file_path)

    if not files_to_track:
        return

    trackable = [f for f in files_to_track if should_track(f, project_dir)]
    if not trackable:
        return

    dirty_file = Path(project_dir) / ".claude" / "mark42" / "dirty-files"
    dirty_file.parent.mkdir(parents=True, exist_ok=True)

    existing: dict[str, str] = {}
    if dirty_file.exists():
        with open(dirty_file) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                path = line.split(" [")[0] if " [" in line else line
                existing[path] = line

    for file_path in trackable:
        if commit_context:
            ctx = f"[{commit_context['hash']}: {commit_context['message']}]"
            existing[file_path] = f"{file_path} {ctx}"
        elif file_path not in existing:
            existing[file_path] = file_path

    with open(dirty_file, "w") as f:
        for line in existing.values():
            f.write(line + "\n")

    # Write session event (JSON Lines) for capture at session end
    event_file = Path(project_dir) / ".claude" / "mark42" / "session-events"
    event_file.parent.mkdir(parents=True, exist_ok=True)
    import datetime
    event = {"toolName": tool_name, "timestamp": datetime.datetime.now(datetime.timezone.utc).isoformat()}
    if tool_name in ("Edit", "Write") and trackable:
        event["filePath"] = trackable[0]
    elif tool_name == "Bash" and command:
        event["command"] = command[:200]
    with open(event_file, "a") as f:
        f.write(json.dumps(event) + "\n")

    # NO output - zero token cost


if __name__ == "__main__":
    main()
