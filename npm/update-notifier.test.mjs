import { test, expect } from 'bun:test';
import { mkdtempSync, readFileSync, writeFileSync, rmSync, existsSync, utimesSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createServer, get } from 'node:http';
import { CHECK_INTERVAL, prepareUpdateNotification, fetchLatestVersion, newerVersion } from '../bin/update-notifier.cjs';

const now = 1800000000000;
function fixture(fn) {
  const directory = mkdtempSync(join(tmpdir(), 'kpm-notifier-'));
  try { return fn(directory); } finally { rmSync(directory, { recursive: true, force: true }); }
}
function options(directory, overrides = {}) {
  return { currentVersion: '0.2.0', interactive: true, env: {}, now, cacheDirectory: directory, startCheck() {}, ...overrides };
}
function cache(directory, value) { writeFileSync(join(directory, 'update.json'), JSON.stringify(value)); }

test('compares stable versions numerically and rejects untrusted strings', () => {
  expect(newerVersion('0.10.0', '0.2.0')).toBe(true);
  expect(newerVersion('1.0.0', '0.99.99')).toBe(true);
  expect(newerVersion('0.2.0', '0.2.0')).toBe(false);
  expect(newerVersion('0.1.99', '0.2.0')).toBe(false);
  expect(newerVersion('0.2.0+build.2', '0.2.0+build.1')).toBe(false);
  for (const version of ['dev', '1.0.0-beta.1', '1.0.0\n\x1b[2J', '01.0.0', null, {}, '']) {
    expect(newerVersion(version, '0.2.0')).toBe(false);
  }
});

test('starts a background attempt once daily, including failed/offline attempts', () => fixture(directory => {
  let checks = 0;
  const opts = options(directory, { startCheck() { checks++; throw new Error('offline'); } });
  expect(prepareUpdateNotification(opts)).toBe('');
  expect(checks).toBe(1);
  expect(prepareUpdateNotification(opts)).toBe('');
  expect(checks).toBe(1);
  expect(prepareUpdateNotification({ ...opts, now: now + CHECK_INTERVAL })).toBe('');
  expect(checks).toBe(2);
}));

test('uses only cached results for notices and avoids repeated notices', () => fixture(directory => {
  cache(directory, { latest: '0.3.0', checkedAt: now });
  let checks = 0;
  const opts = options(directory, { startCheck() { checks++; } });
  const notice = prepareUpdateNotification(opts);
  expect(notice).toContain('KPM 0.3.0 is available (installed: 0.2.0)');
  expect(notice).toContain('npm install -g @composablehorizons/kpm@latest');
  expect(checks).toBe(0);
  expect(prepareUpdateNotification(opts)).toBe('');
  expect(prepareUpdateNotification({ ...opts, now: now + CHECK_INTERVAL })).toContain('0.3.0');
  expect(checks).toBe(1);
}));

test('shows a newly discovered version even if another version was notified today', () => fixture(directory => {
  cache(directory, { latest: '0.4.0', checkedAt: now, notifiedAt: now, notifiedVersion: '0.3.0' });
  expect(prepareUpdateNotification(options(directory))).toContain('0.4.0');
}));

test('silently skips CI, noninteractive runs, opt-outs, and development builds', () => fixture(directory => {
  cache(directory, { latest: '0.3.0', checkedAt: 0 });
  for (const override of [
    { interactive: false }, { env: { CI: 'true' } }, { env: { KPM_NO_UPDATE_NOTIFIER: '1' } },
    { env: { NO_UPDATE_NOTIFIER: '1' } }, { env: { npm_config_update_notifier: 'false' } },
    { currentVersion: 'dev' },
  ]) {
    expect(prepareUpdateNotification(options(directory, { ...override, startCheck() { throw new Error('must not run'); } }))).toBe('');
  }
  expect(JSON.parse(readFileSync(join(directory, 'update.json'))).checkedAt).toBe(0);
}));

test('handles corrupt caches, unwritable directories, and another running check', () => fixture(directory => {
  writeFileSync(join(directory, 'update.json'), '{broken');
  expect(prepareUpdateNotification(options(directory))).toBe('');
  expect(existsSync(join(directory, 'update.lock'))).toBe(true);
  // Use the same clock for the fixture and filesystem, including sub-ms mtimes.
  utimesSync(join(directory, 'update.lock'), new Date(now), new Date(now));
  // A fresh lock prevents a second process from spawning another worker.
  cache(directory, { checkedAt: 0 });
  let checks = 0;
  expect(prepareUpdateNotification(options(directory, { startCheck() { checks++; } }))).toBe('');
  expect(checks).toBe(0);
  writeFileSync(join(directory, 'not-a-directory'), '');
  expect(prepareUpdateNotification(options(join(directory, 'not-a-directory')))).toBe('');
}));

test('recovers an expired lock and timestamps from a future system clock', () => fixture(directory => {
  writeFileSync(join(directory, 'update.lock'), '');
  cache(directory, { checkedAt: now + CHECK_INTERVAL * 10 });
  let checks = 0;
  prepareUpdateNotification(options(directory, { now: Date.now() + 60000, startCheck() { checks++; } }));
  expect(checks).toBe(1);
}));

async function withRegistry(handler, run) {
  const server = createServer(handler);
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const url = `http://127.0.0.1:${server.address().port}/latest`;
  try { await run(url); } finally { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); }
}

test('reads latest version metadata and silently handles HTTP and JSON failures', async () => {
  for (const [status, body, expected] of [
    [200, '{"version":"0.3.0"}', '0.3.0'], [503, '{}', null], [200, 'broken', null],
    [200, '{"version":"0.3.0-beta.1"}', null], [200, '{}', null],
    [200, 'x'.repeat(70000), null],
  ]) {
    await withRegistry((req, res) => { res.writeHead(status); res.end(body); }, async url => {
      expect(await fetchLatestVersion({ request: get, url })).toBe(expected);
    });
  }
});

test('bounds the worker network request with a deadline even for a stalled server', async () => {
  await withRegistry(() => {}, async url => {
    const started = Date.now();
    expect(await fetchLatestVersion({ request: get, url, timeoutMs: 30 })).toBe(null);
    expect(Date.now() - started).toBeLessThan(1000);
  });
  expect(await fetchLatestVersion({ request() { throw new Error('offline'); } })).toBe(null);
});
