#!/usr/bin/env node
// Stages the npm packages for a release from its archives.
//
//   node packaging/npm/stage.mjs --version v0.1.0 --dist dist --out dist/npm
//
// For every platform archive (dist/prodshadow_<version>_<goos>_<goarch>.tar.gz)
// it writes a package prodshadow-<os>-<cpu> holding the binary, with npm's
// "os" and "cpu" fields so only the matching one is installed, and then the
// main package prodshadow, whose launcher runs that binary. The directories
// are printed in publish order: platform packages first.
//
// --sums FILE checks every archive against a SHA256SUMS file first.
// --allow-missing stages only the platforms whose archive exists (for local
// tests; a release needs all of them).

import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");

const PLATFORMS = [
  { goos: "darwin", goarch: "arm64", os: "darwin", cpu: "arm64", label: "macOS on Apple silicon" },
  { goos: "darwin", goarch: "amd64", os: "darwin", cpu: "x64", label: "macOS on Intel" },
  { goos: "linux", goarch: "arm64", os: "linux", cpu: "arm64", label: "Linux on arm64" },
  { goos: "linux", goarch: "amd64", os: "linux", cpu: "x64", label: "Linux on x64" },
];

const { values } = parseArgs({
  options: {
    version: { type: "string" },
    dist: { type: "string", default: "dist" },
    out: { type: "string", default: "dist/npm" },
    sums: { type: "string" },
    "allow-missing": { type: "boolean", default: false },
  },
});

function die(message) {
  process.stderr.write(`stage.mjs: ${message}\n`);
  process.exit(1);
}

const version = (values.version || "").replace(/^v/, "");
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  die(`--version must be a semantic version such as v0.1.0, got "${values.version || ""}"`);
}

const mainTemplate = JSON.parse(
  fs.readFileSync(path.join(here, "prodshadow", "package.json"), "utf8"),
);
const checksums = values.sums ? readSums(values.sums) : null;

fs.rmSync(values.out, { recursive: true, force: true });
fs.mkdirSync(values.out, { recursive: true });

const staged = [];
const optional = {};
for (const p of PLATFORMS) {
  const archive = `prodshadow_${version}_${p.goos}_${p.goarch}.tar.gz`;
  const archivePath = path.join(values.dist, archive);
  if (!fs.existsSync(archivePath)) {
    if (values["allow-missing"]) {
      continue;
    }
    die(`missing ${archivePath}; a release needs every platform (or pass --allow-missing)`);
  }
  if (checksums) {
    const want = checksums.get(archive);
    const got = createHash("sha256").update(fs.readFileSync(archivePath)).digest("hex");
    if (!want) die(`${archive} is not listed in ${values.sums}`);
    if (want !== got) die(`${archive}: checksum ${got} does not match ${want}`);
  }

  const name = `prodshadow-${p.os}-${p.cpu}`;
  const dir = path.join(values.out, name);
  fs.mkdirSync(path.join(dir, "bin"), { recursive: true });
  const binary = extractBinary(archivePath);
  fs.copyFileSync(binary, path.join(dir, "bin", "prodshadow"));
  fs.chmodSync(path.join(dir, "bin", "prodshadow"), 0o755);
  fs.copyFileSync(path.join(root, "LICENSE"), path.join(dir, "LICENSE"));
  writeJSON(path.join(dir, "package.json"), {
    name,
    version,
    description: `The prodshadow binary for ${p.label}. Install the prodshadow package instead of this one.`,
    homepage: mainTemplate.homepage,
    repository: mainTemplate.repository,
    license: mainTemplate.license,
    author: mainTemplate.author,
    os: [p.os],
    cpu: [p.cpu],
    files: ["bin/prodshadow"],
    preferUnplugged: true,
  });
  fs.writeFileSync(
    path.join(dir, "README.md"),
    `# ${name}\n\nThe \`prodshadow\` ${version} binary for ${p.label}.\n\n` +
      "Install [`prodshadow`](https://www.npmjs.com/package/prodshadow) instead: it depends on\n" +
      "this package on matching machines and runs the binary for you.\n",
  );
  optional[name] = version;
  staged.push(dir);
}
if (staged.length === 0) {
  die(`no archives for ${version} in ${values.dist}`);
}

const mainDir = path.join(values.out, "prodshadow");
fs.mkdirSync(path.join(mainDir, "bin"), { recursive: true });
fs.copyFileSync(
  path.join(here, "prodshadow", "bin", "prodshadow.js"),
  path.join(mainDir, "bin", "prodshadow.js"),
);
fs.chmodSync(path.join(mainDir, "bin", "prodshadow.js"), 0o755);
fs.copyFileSync(path.join(here, "prodshadow", "README.md"), path.join(mainDir, "README.md"));
fs.copyFileSync(path.join(root, "LICENSE"), path.join(mainDir, "LICENSE"));
writeJSON(path.join(mainDir, "package.json"), {
  ...mainTemplate,
  version,
  optionalDependencies: optional,
});
staged.push(mainDir);

process.stdout.write(staged.join("\n") + "\n");

function readSums(file) {
  const sums = new Map();
  for (const line of fs.readFileSync(file, "utf8").split("\n")) {
    const m = line.match(/^([0-9a-f]{64})\s+\*?(\S+)$/);
    if (m) sums.set(m[2], m[1]);
  }
  return sums;
}

function extractBinary(archivePath) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "prodshadow-npm-"));
  execFileSync("tar", ["-xzf", archivePath, "-C", tmp]);
  const top = fs.readdirSync(tmp);
  if (top.length !== 1) die(`${archivePath} should contain one directory, found ${top.join(", ")}`);
  const binary = path.join(tmp, top[0], "prodshadow");
  if (!fs.existsSync(binary)) die(`${archivePath} has no prodshadow binary`);
  return binary;
}

function writeJSON(file, value) {
  fs.writeFileSync(file, JSON.stringify(value, null, 2) + "\n");
}
