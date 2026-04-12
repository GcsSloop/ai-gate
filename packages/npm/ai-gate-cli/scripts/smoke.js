const { spawnSync } = require("node:child_process");
const path = require("node:path");

const bin = path.join(__dirname, "..", "bin", "ai-gate.js");

function run(args) {
  const res = spawnSync(process.execPath, [bin, ...args], { encoding: "utf8" });
  if (res.status !== 0) {
    console.error(res.stdout);
    console.error(res.stderr);
    throw new Error(`command failed: ai-gate ${args.join(" ")}`);
  }
}

run(["--help"]);
run(["--version"]);
run(["skill", "--help"]);
run(["skill", "add", "github:openai/skills:main:code-review"]);

console.log("smoke passed");
