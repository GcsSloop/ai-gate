# ai-gate CLI (npm placeholder)

This directory is an isolated npm package skeleton for publishing the `ai-gate` command.

## Current status

- Publishable placeholder package
- Includes executable `ai-gate` command
- Includes placeholder `skill` subcommands

## Local verification

```bash
cd packages/npm/ai-gate-cli
npm run smoke
npm run pack:check
```

## Publish

```bash
cd packages/npm/ai-gate-cli
npm publish
```

If the package name `ai-gate` is unavailable, update `name` in `package.json` (for example, to a scoped package), then publish with:

```bash
npm publish --access public
```
