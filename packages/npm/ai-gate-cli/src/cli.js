function printRootHelp() {
  console.log("ai-gate CLI (placeholder)");
  console.log("");
  console.log("Usage:");
  console.log("  ai-gate skill <subcommand> [options]");
  console.log("");
  console.log("Skill subcommands:");
  console.log("  add <skill-ref>    Install a skill by discovered id or repository reference");
  console.log("  list               List installed skills (placeholder)");
  console.log("  remove <name>      Remove an installed skill (placeholder)");
  console.log("");
  console.log("Examples:");
  console.log("  ai-gate skill add github:openai/skills:main:code-review");
  console.log("  ai-gate skill add https://github.com/openai/skills --skill code-review");
}

function printSkillHelp() {
  console.log("Usage:");
  console.log("  ai-gate skill add <skill-ref> [--skill <name>] [--agent <name>] [--global]");
  console.log("  ai-gate skill list");
  console.log("  ai-gate skill remove <name>");
}

function parseFlags(args) {
  const flags = {};
  const positionals = [];
  for (let i = 0; i < args.length; i += 1) {
    const token = args[i];
    if (!token.startsWith("--")) {
      positionals.push(token);
      continue;
    }
    const key = token.slice(2);
    const next = args[i + 1];
    if (!next || next.startsWith("--")) {
      flags[key] = true;
      continue;
    }
    flags[key] = next;
    i += 1;
  }
  return { flags, positionals };
}

function runSkill(args) {
  if (args.length === 0 || args[0] === "--help" || args[0] === "-h") {
    printSkillHelp();
    return 0;
  }
  const sub = args[0];
  const parsed = parseFlags(args.slice(1));
  if (sub === "add") {
    const skillRef = parsed.positionals[0];
    if (!skillRef) {
      console.error("Missing <skill-ref>.");
      printSkillHelp();
      return 1;
    }
    const skillName = parsed.flags.skill || "";
    const agent = parsed.flags.agent || "codex";
    const installGlobal = Boolean(parsed.flags.global);
    console.log("ai-gate skill add (placeholder)");
    console.log(`  skill-ref: ${skillRef}`);
    if (skillName) console.log(`  skill: ${skillName}`);
    console.log(`  agent: ${agent}`);
    console.log(`  global: ${installGlobal}`);
    console.log("");
    console.log("Next step: implement real downloader + installer in this package.");
    return 0;
  }
  if (sub === "list") {
    console.log("ai-gate skill list (placeholder)");
    return 0;
  }
  if (sub === "remove") {
    const name = parsed.positionals[0];
    if (!name) {
      console.error("Missing <name>.");
      printSkillHelp();
      return 1;
    }
    console.log(`ai-gate skill remove (placeholder): ${name}`);
    return 0;
  }
  console.error(`Unknown skill subcommand: ${sub}`);
  printSkillHelp();
  return 1;
}

function run(args) {
  if (args.length === 0 || args[0] === "--help" || args[0] === "-h") {
    printRootHelp();
    return 0;
  }
  if (args[0] === "--version" || args[0] === "-v") {
    const pkg = require("../package.json");
    console.log(pkg.version);
    return 0;
  }
  const top = args[0];
  if (top === "skill") {
    return runSkill(args.slice(1));
  }
  console.error(`Unknown command: ${top}`);
  printRootHelp();
  return 1;
}

module.exports = {
  run,
};
