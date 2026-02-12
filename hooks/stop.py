#!/usr/bin/env python3
"""Stop hook - terse memory sync with session capture."""

import json
import os
import sys
from pathlib import Path


def main():
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if not project_dir:
        return

    # Guard: only fire once per session (prevents infinite loop)
    flag_file = Path(project_dir) / ".claude" / "mark42" / "stop-prompted"
    if flag_file.exists():
        return
    flag_file.parent.mkdir(parents=True, exist_ok=True)
    flag_file.touch()

    # Read session input from Claude Code
    try:
        session_data = json.loads(sys.stdin.read())
    except (json.JSONDecodeError, Exception):
        session_data = {}

    project_name = Path(project_dir).name

    # Gather dirty files
    files = []
    dirty_file = Path(project_dir) / ".claude" / "mark42" / "dirty-files"
    if dirty_file.exists() and dirty_file.stat().st_size > 0:
        files = [f.strip() for f in dirty_file.read_text().split("\n") if f.strip()][:5]

    # Read session events
    events = []
    event_file = Path(project_dir) / ".claude" / "mark42" / "session-events"
    if event_file.exists() and event_file.stat().st_size > 0:
        for line in event_file.read_text().strip().split("\n"):
            line = line.strip()
            if line:
                try:
                    events.append(json.loads(line))
                except json.JSONDecodeError:
                    pass

    files_list = ", ".join(files) if files else "none"
    event_count = len(events)

    # Build events JSON for capture_session
    events_json = json.dumps(events[:50])  # Cap at 50 events

    output = {
        "decision": "block",
        "reason": (
            f"Memory sync for {project_name}. "
            f"Modified: {files_list}. Events: {event_count}. "
            f"1. Call capture_session with projectName='{project_name}', "
            f"a brief summary of what was done, and events={events_json}. "
            f"2. Use create_or_update_entities for new learnings, add_observations for updates. "
            f"Use fact_type='static' for conventions/patterns, 'dynamic' for decisions/context. "
            f"Reply only: 'Synced N learnings.'"
        ),
        "suppressOutput": True,
    }
    print(json.dumps(output))

    # Clear buffers
    if dirty_file.exists():
        dirty_file.write_text("")
    if event_file.exists():
        event_file.write_text("")


if __name__ == "__main__":
    main()
