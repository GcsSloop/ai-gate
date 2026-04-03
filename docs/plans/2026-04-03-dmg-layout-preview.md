# DMG Layout Preview Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a styled macOS DMG layout with a custom background image, fixed 800x500 window, and app/Application shortcut placement, then generate a local preview build.

**Architecture:** Keep Tauri's existing app build output, then post-process the DMG on macOS using a repo-owned shell script plus Finder AppleScript. The script will stage a temporary writable image, copy the background asset into a hidden folder, set Finder view/layout metadata, convert to the final compressed DMG, and leave the existing signing/notarization path compatible.

**Tech Stack:** Bash, AppleScript, hdiutil, Finder metadata, local Tauri macOS bundle output.

---

### Task 1: Add a reusable DMG styling script
**Files:**
- Create: `scripts/desktop/style_dmg.sh`
- Modify: `scripts/desktop/notarize_macos.sh`
- Test: local shell execution on macOS bundle output

**Step 1:** Write the shell script to mount a writable DMG, create `.background`, copy `assets/dmg-bg.jpg`, add `/Applications` symlink, configure Finder window size/background/icon size/icon positions with AppleScript, detach, and convert back to compressed DMG.

**Step 2:** Wire the notarization pipeline to call the styling script after rebuilding the raw DMG and before DMG signing.

**Step 3:** Keep the DMG signing step after styling so the final DMG signature matches the shipped artifact.

### Task 2: Produce a local preview build
**Files:**
- Modify: local build outputs under `desktop/src-tauri/target/universal-apple-darwin/release/bundle`

**Step 1:** Build or restyle a local DMG using the new script.

**Step 2:** Mount the DMG locally and capture a screenshot for visual review.

**Step 3:** Adjust icon coordinates if the screenshot does not match the intended empty regions.
