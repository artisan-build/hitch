#!/usr/bin/env node
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawnSync } = require('child_process');
const https = require('https');
const crypto = require('crypto');
const tar = require('tar');

const pkg = require('../package.json');
const repo = process.env.HITCH_REPO || 'artisan-build/hitch';
const version = process.env.HITCH_VERSION || `v${pkg.version}`;

function fail(message) {
  console.error(`hitch-mcp: ${message}`);
  process.exit(1);
}

function platformParts() {
  const osPart = process.platform === 'darwin' || process.platform === 'linux' ? process.platform : null;
  const archPart = process.arch === 'x64' ? 'amd64' : process.arch === 'arm64' ? 'arm64' : null;
  if (!osPart || !archPart) {
    fail(`unsupported platform ${process.platform}/${process.arch}`);
  }
  return { osPart, archPart };
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest, { mode: 0o600 });
    https.get(url, (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        response.resume();
        file.close(() => download(response.headers.location, dest).then(resolve, reject));
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        file.close(() => fs.rmSync(dest, { force: true }));
        reject(new Error(`download failed with HTTP ${response.statusCode}: ${url}`));
        return;
      }
      response.pipe(file);
      file.on('finish', () => file.close(resolve));
    }).on('error', reject);
  });
}

async function main() {
  const { osPart, archPart } = platformParts();
  const asset = `hitch_${osPart}_${archPart}.tar.gz`;
  const base = `https://github.com/${repo}/releases/download/${version}`;
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'hitch-mcp-'));
  const archive = path.join(tmp, asset);
  const checksums = path.join(tmp, 'checksums.txt');
  try {
    await download(`${base}/${asset}`, archive);
    await download(`${base}/checksums.txt`, checksums);
    const expectedLine = fs.readFileSync(checksums, 'utf8').split('\n').find((line) => line.trim().endsWith(`  ${asset}`));
    if (!expectedLine) {
      fail(`checksums.txt does not contain ${asset}`);
    }
    const expected = expectedLine.trim().split(/\s+/)[0];
    const actual = crypto.createHash('sha256').update(fs.readFileSync(archive)).digest('hex');
    if (actual !== expected) {
      fail(`checksum mismatch for ${asset}`);
    }
    await tar.x({ file: archive, cwd: tmp });
    const hitch = path.join(tmp, 'hitch');
    fs.chmodSync(hitch, 0o755);
    const result = spawnSync(hitch, ['install', ...process.argv.slice(2)], { stdio: 'inherit' });
    process.exit(result.status == null ? 1 : result.status);
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

main().catch((error) => fail(error.message));
