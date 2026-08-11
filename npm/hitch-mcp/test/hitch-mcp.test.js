'use strict';
const { test } = require('node:test');
const assert = require('node:assert');
const http = require('node:http');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');
const tar = require('tar');

const shimPath = path.join(__dirname, '..', 'bin', 'hitch-mcp.js');
const osPart = process.platform;
const archPart = process.arch === 'x64' ? 'amd64' : 'arm64';
const asset = `hitch_${osPart}_${archPart}.tar.gz`;

// A stand-in for the released binary: proves it was extracted, made executable,
// and actually spawned by writing a marker file the test can check.
const fakeHitch = '#!/bin/sh\necho "$1" > "$HITCH_TEST_MARKER"\nexit 0\n';

function buildFixture(dir) {
  fs.writeFileSync(path.join(dir, 'hitch'), fakeHitch);
  const archive = path.join(dir, asset);
  tar.create({ gzip: true, file: archive, cwd: dir, sync: true }, ['hitch']);
  const bytes = fs.readFileSync(archive);
  const digest = crypto.createHash('sha256').update(bytes).digest('hex');
  return { bytes, digest };
}

function startServer(routes) {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const route = routes[req.url];
      if (!route) {
        res.writeHead(404);
        res.end('not found');
        return;
      }
      route(req, res);
    });
    server.listen(0, '127.0.0.1', () => resolve(server));
  });
}

// Must be an async spawn: a sync spawn would block the event loop, and the
// test's HTTP server (same process) could never answer the shim's requests.
function runShim(baseUrl, workDir, timeoutMs = 30000) {
  const tmpBase = path.join(workDir, 'shim-tmp');
  const marker = path.join(workDir, 'marker.txt');
  fs.mkdirSync(tmpBase);
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [shimPath], {
      env: {
        ...process.env,
        HITCH_BASE_URL: baseUrl,
        TMPDIR: tmpBase,
        HITCH_TEST_MARKER: marker,
      },
    });
    // A hung shim (e.g. an unbounded redirect loop) is killed, surfacing as
    // status null with empty stderr rather than blocking the test run.
    const killTimer = setTimeout(() => child.kill('SIGKILL'), timeoutMs);
    let stderr = '';
    child.stdout.resume();
    child.stderr.on('data', (chunk) => { stderr += chunk; });
    child.on('error', reject);
    child.on('close', (status) => {
      clearTimeout(killTimer);
      resolve({ result: { status, stderr }, tmpBase, marker });
    });
  });
}

function withCase(t, fn) {
  const workDir = fs.mkdtempSync(path.join(os.tmpdir(), 'hitch-shim-test-'));
  t.after(() => fs.rmSync(workDir, { recursive: true, force: true }));
  return fn(workDir);
}

test('follows a 302 redirect, verifies the checksum, and leaves an executable binary', (t) =>
  withCase(t, async (workDir) => {
    const { bytes, digest } = buildFixture(workDir);
    const server = await startServer({
      [`/dl/${asset}`]: (req, res) => {
        const { port } = server.address();
        res.writeHead(302, { location: `http://127.0.0.1:${port}/real/${asset}` });
        res.end();
      },
      [`/real/${asset}`]: (req, res) => {
        res.writeHead(200);
        res.end(bytes);
      },
      '/dl/checksums.txt': (req, res) => {
        res.writeHead(200);
        res.end(`${digest}  ${asset}\n`);
      },
    });
    t.after(() => server.close());
    const base = `http://127.0.0.1:${server.address().port}/dl`;
    const { result, tmpBase, marker } = await runShim(base, workDir);
    assert.strictEqual(result.status, 0, `shim exited ${result.status}: ${result.stderr}`);
    assert.strictEqual(fs.readFileSync(marker, 'utf8'), 'install\n', 'extracted binary did not run');
    assert.deepStrictEqual(fs.readdirSync(tmpBase), [], 'shim left files in its temp dir');
  }));

test('exits non-zero on 404 without leaving a half-written file', (t) =>
  withCase(t, async (workDir) => {
    const server = await startServer({});
    t.after(() => server.close());
    const base = `http://127.0.0.1:${server.address().port}/dl`;
    const { result, tmpBase, marker } = await runShim(base, workDir);
    assert.notStrictEqual(result.status, 0, 'shim exited 0 on a 404');
    assert.match(result.stderr, /HTTP 404/);
    assert.ok(!fs.existsSync(marker), 'binary ran despite failed download');
    assert.deepStrictEqual(fs.readdirSync(tmpBase), [], 'shim left files behind after 404');
  }));

test('exits non-zero on a redirect loop instead of hanging', (t) =>
  withCase(t, async (workDir) => {
    const server = await startServer({
      [`/dl/${asset}`]: (req, res) => {
        const { port } = server.address();
        res.writeHead(302, { location: `http://127.0.0.1:${port}/dl/${asset}` });
        res.end();
      },
    });
    t.after(() => server.close());
    const base = `http://127.0.0.1:${server.address().port}/dl`;
    // Short timeout: if the redirect cap regresses, the shim hangs and the
    // kill timer turns that into status null + empty stderr, failing both
    // asserts below instead of blocking CI.
    const { result, tmpBase, marker } = await runShim(base, workDir, 10000);
    assert.ok(result.status !== null && result.status !== 0, `expected a clean non-zero exit, got status ${result.status}`);
    assert.match(result.stderr, /hitch-mcp: /, 'shim did not fail with a clear message');
    assert.ok(!fs.existsSync(marker), 'binary ran despite redirect loop');
    assert.deepStrictEqual(fs.readdirSync(tmpBase), [], 'shim left files behind after redirect loop');
  }));

test('exits non-zero on checksum mismatch with nothing extracted', (t) =>
  withCase(t, async (workDir) => {
    const { bytes } = buildFixture(workDir);
    const wrongDigest = '0'.repeat(64);
    const server = await startServer({
      [`/dl/${asset}`]: (req, res) => {
        res.writeHead(200);
        res.end(bytes);
      },
      '/dl/checksums.txt': (req, res) => {
        res.writeHead(200);
        res.end(`${wrongDigest}  ${asset}\n`);
      },
    });
    t.after(() => server.close());
    const base = `http://127.0.0.1:${server.address().port}/dl`;
    const { result, tmpBase, marker } = await runShim(base, workDir);
    assert.notStrictEqual(result.status, 0, 'shim exited 0 on checksum mismatch');
    assert.match(result.stderr, /checksum mismatch/);
    assert.ok(!fs.existsSync(marker), 'binary ran despite checksum mismatch');
    assert.deepStrictEqual(fs.readdirSync(tmpBase), [], 'shim left extracted files after checksum mismatch');
  }));
