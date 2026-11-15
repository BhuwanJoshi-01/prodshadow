#!/usr/bin/env node
// Runs the prodshadow binary that npm installed with the platform package
// matching this machine (prodshadow-<os>-<cpu>, an optional dependency).
// Nothing is downloaded at install time and no install scripts run.
"use strict";

const { spawn } = require("node:child_process");

const PLATFORMS = {
  "darwin-arm64": "prodshadow-darwin-arm64",
  "darwin-x64": "prodshadow-darwin-x64",
  "linux-arm64": "prodshadow-linux-arm64",
  "linux-x64": "prodshadow-linux-x64",
};

function fail(message) {
  process.stderr.write(`prodshadow: ${message}\n`);
  process.exit(1);
}

function binaryPath() {
  if (process.env.PRODSHADOW_BINARY) {
    return process.env.PRODSHADOW_BINARY;
  }
  const key = `${process.platform}-${process.arch}`;
  const pkg = PLATFORMS[key];
  if (!pkg) {
    fail(
      `there is no prebuilt binary for ${key} (available: ${Object.keys(PLATFORMS).join(", ")}).\n` +
        "On Windows, install it inside WSL. Elsewhere, build it with Go:\n" +
        "  go install github.com/BhuwanJoshi-01/prodshadow/cmd/prodshadow@latest",
    );
  }
  try {
    return require.resolve(`${pkg}/bin/prodshadow`);
  } catch {
    return fail(
      `the package ${pkg}, which holds the binary for ${key}, is not installed.\n` +
        "It is an optional dependency of prodshadow; reinstall without --no-optional or --omit=optional.",
    );
  }
}

const child = spawn(binaryPath(), process.argv.slice(2), { stdio: "inherit" });

// prodshadow removes its containers when interrupted, so signals are passed
// on and this process waits for it to finish instead of exiting first.
for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  process.on(signal, () => {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill(signal);
    }
  });
}

child.on("error", (err) => fail(`could not run ${child.spawnfile}: ${err.message}`));
child.on("exit", (code, signal) => {
  if (signal) {
    process.removeAllListeners(signal);
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code === null ? 1 : code);
});
