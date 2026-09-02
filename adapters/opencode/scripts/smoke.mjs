import {
  mkdtempSync,
  writeFileSync,
  chmodSync,
  readFileSync,
  existsSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { Mark42 } from "../mark42.js";

// Fake `mark42` on PATH: records argv + stdin, and (for capture) writes a
// current-session file so the follow-up distill can resolve the session name.
const tmp = mkdtempSync(path.join(os.tmpdir(), "mark42-smoke-"));
const logFile = path.join(tmp, "invocations.jsonl");

const fake = path.join(tmp, "mark42");
writeFileSync(
  fake,
  `#!/usr/bin/env node
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
let stdin = "";
process.stdin.on("data", (d) => (stdin += d));
process.stdin.on("end", () => {
  const argv = process.argv.slice(2);
  fs.appendFileSync(process.env.SMOKE_LOG, JSON.stringify({ argv, stdin }) + "\\n");
  if (argv[0] === "session" && argv[1] === "capture") {
    const slug = process.env.CLAUDE_PROJECT_DIR.replaceAll("/", "-");
    const dir = path.join(os.homedir(), ".mark42", "state", slug);
    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(path.join(dir, "current-session"), "session-smoke-test");
  }
});
`,
);
chmodSync(fake, 0o755);

process.env.PATH = `${tmp}:${process.env.PATH}`;
process.env.SMOKE_LOG = logFile;
process.env.HOME = tmp;

const ctx = {
  project: { name: "smoke-test" },
  client: {},
  $: async () => {},
  directory: "/tmp/smoke-project",
  worktree: null,
};

const plugin = await Mark42(ctx);

await plugin.event({
  event: { type: "session.created", properties: { info: { id: "s1" } } },
});
await plugin["tool.execute.after"](
  { tool: "bash", args: { command: "go test ./..." } },
  {},
);
await plugin["tool.execute.after"](
  { tool: "edit", args: { filePath: "/tmp/smoke-project/main.go" } },
  {},
);
await plugin["tool.execute.after"](
  { tool: "grep", args: { path: "/tmp/smoke-project/src" } },
  {},
);
await plugin.event({
  event: { type: "session.idle", properties: { sessionID: "s1" } },
});

// Wait for the fire-and-forget spawns to finish (poll up to 2s for distill,
// which is chained on capture's close event).
let lines = [];
for (let i = 0; i < 40; i++) {
  if (existsSync(logFile)) {
    lines = readFileSync(logFile, "utf8")
      .trim()
      .split("\n")
      .filter(Boolean)
      .map((l) => JSON.parse(l));
    if (lines.some((l) => l.argv[0] === "distill")) break;
  }
  await new Promise((r) => setTimeout(r, 50));
}

const capture = lines.find(
  (l) => l.argv[0] === "session" && l.argv[1] === "capture",
);
const distill = lines.find((l) => l.argv[0] === "distill");

if (!capture) throw new Error("capture was not spawned");
const payload = JSON.parse(capture.stdin);
if (typeof payload.summary !== "string" || !Array.isArray(payload.events)) {
  throw new Error("capture stdin JSON does not match {summary, events} schema");
}
if (payload.events.length !== 3) {
  throw new Error(`expected 3 events, got ${payload.events.length}`);
}
const names = payload.events.map((e) => e.toolName);
if (names.join(",") !== "Bash,Edit,Grep") {
  throw new Error(`unexpected tool mapping: ${names.join(",")}`);
}
if (!distill || distill.argv[1] !== "session-smoke-test") {
  throw new Error("distill not spawned with the resolved session name");
}

console.log("PASS: dispatched events, capture + distill spawned correctly");
console.log("  capture argv:", capture.argv.join(" "));
console.log("  capture events:", names.join(", "));
console.log("  distill argv:", distill.argv.join(" "));
