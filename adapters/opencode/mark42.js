import { spawn } from "node:child_process";
import { homedir } from "node:os";
import path from "node:path";
import fs from "node:fs";

const QUEUE_CAP = 500;

const TOOL_MAP = {
  bash: "Bash",
  edit: "Edit",
  write: "Write",
  read: "Read",
  grep: "Grep",
};

function projectSlug(projectDir) {
  return projectDir.replaceAll("/", "-");
}

function stateDir(projectDir) {
  const home = process.env.HOME || homedir();
  return path.join(home, ".mark42", "state", projectSlug(projectDir));
}

function currentSessionPath(projectDir) {
  return path.join(stateDir(projectDir), "current-session");
}

function errorLogPath() {
  const home = process.env.HOME || homedir();
  return path.join(home, ".mark42", "state", "adapter-errors.log");
}

function logError(message) {
  console.warn(`[mark42] ${message}`);
  try {
    fs.mkdirSync(path.dirname(errorLogPath()), { recursive: true });
    fs.appendFileSync(
      errorLogPath(),
      `[${new Date().toISOString()}] ${message}\n`,
    );
  } catch {
    // never throw
  }
}

function extractFilePath(tool, args) {
  if (!args) return "";
  if (tool === "grep") return args.path ?? "";
  return args.filePath ?? "";
}

export const Mark42 = async ({ project, client, $, directory, worktree }) => {
  const projectDir = directory || process.cwd();
  const projectName = project?.name || path.basename(projectDir);

  const queue = [];
  let flushed = false;

  function mapTool(tool, args) {
    const toolName = TOOL_MAP[tool] || "Other";
    const filePath = extractFilePath(tool, args);
    let command = "";
    if (tool === "bash" && typeof args?.command === "string") {
      command = args.command.trim();
      if (command.length > 200) command = command.slice(0, 200);
    }
    return { toolName, filePath, command };
  }

  function enqueue(event) {
    if (queue.length >= QUEUE_CAP) queue.shift();
    queue.push(event);
  }

  function readCurrentSession() {
    try {
      return fs.readFileSync(currentSessionPath(projectDir), "utf8").trim();
    } catch {
      return "";
    }
  }

  function spawnDistill(sessionName) {
    const child = spawn("mark42", ["distill", sessionName], {
      stdio: "ignore",
    });
    child.on("error", (err) =>
      logError(`distill spawn failed: ${err.message}`),
    );
  }

  function spawnCapture(events) {
    const summary = `OpenCode session: ${events.length} tool events`;
    const payload = JSON.stringify({ summary, events });

    const child = spawn("mark42", ["session", "capture", projectName], {
      env: { ...process.env, CLAUDE_PROJECT_DIR: projectDir },
      stdio: ["pipe", "ignore", "ignore"],
    });
    child.on("error", (err) =>
      logError(`capture spawn failed: ${err.message}`),
    );
    child.stdin.on("error", () => {});
    child.stdin.write(payload);
    child.stdin.end();

    child.on("close", () => {
      const sessionName = readCurrentSession();
      if (sessionName) spawnDistill(sessionName);
    });
  }

  function flush() {
    if (flushed) return;
    flushed = true;
    const events = queue.splice(0, queue.length);
    if (events.length === 0) return;
    spawnCapture(events);
  }

  return {
    event: async ({ event }) => {
      try {
        switch (event.type) {
          case "session.created":
            queue.length = 0;
            flushed = false;
            break;
          case "session.idle":
          case "session.error":
            flush();
            break;
        }
      } catch (err) {
        logError(`event handler error: ${err?.message ?? err}`);
      }
    },

    "tool.execute.after": async (input, output) => {
      try {
        const tool = input?.tool ?? "";
        const args = input?.args ?? {};
        enqueue(mapTool(tool, args));
      } catch (err) {
        logError(`tool.execute.after error: ${err?.message ?? err}`);
      }
    },
  };
};
