import { test, expect } from 'bun:test';
import { version } from '../package.json';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
const launcher = fileURLToPath(new URL('../bin/kpm.cjs', import.meta.url));
const run = (args, cwd) => spawnSync(process.execPath, [launcher, ...args], { cwd, encoding: 'utf8' });
test('launcher runs the bundled binary and forwards version arguments', () => {
  const directory = mkdtempSync(join(tmpdir(), 'kpm-launcher-'));
  try {
    writeFileSync(join(directory, 'package.yml'), 'version: 1.2.3\n');
    const result = run(['version', 'patch', '--no-git-tag-version'], directory);
    expect(result.status).toBe(0);
    expect(result.stdout.trim()).toBe('1.2.4');
    expect(run(['version'], directory).stdout.trim()).toBe('1.2.4');
  } finally { rmSync(directory, { recursive: true }); }
});
test('launcher propagates command errors', () => {
  const result = run(['unknown']);
  expect(result.status).toBe(1);
  expect(result.stderr).toContain('unknown command');
});
test('tool version is distinct from the package version', () => {
  expect(run(['--version']).stdout.trim()).toBe(version);
});

test('cached update notices preserve stdout and command exit status', () => {
  const directory = mkdtempSync(join(tmpdir(), 'kpm-notice-launcher-'));
  try {
    const cache = join(directory, 'kpm');
    mkdirSync(cache);
    const preload = join(directory, 'tty.cjs');
    writeFileSync(preload, "Object.defineProperty(process.stdout, 'isTTY', {value: true}); Object.defineProperty(process.stderr, 'isTTY', {value: true});");
    writeFileSync(join(directory, 'package.yml'), 'version: 1.2.3\n');
    const env = { ...process.env, XDG_CACHE_HOME: directory, CI: '', KPM_NO_UPDATE_NOTIFIER: '', NO_UPDATE_NOTIFIER: '', npm_config_update_notifier: 'true' };
    const freshCache = () => writeFileSync(join(cache, 'update.json'), JSON.stringify({ latest: '9999.0.0', checkedAt: Date.now() }));
    const invoke = args => spawnSync('node', ['--require', preload, launcher, ...args], { cwd: directory, encoding: 'utf8', env });
    freshCache();
    const success = invoke(['version']);
    expect(success.status).toBe(0);
    expect(success.stdout).toBe('1.2.3\n');
    expect(success.stderr).toContain('npm install -g @composablehorizons/kpm@latest');
    freshCache();
    const failure = invoke(['unknown']);
    expect(failure.status).toBe(1);
    expect(failure.stderr).toContain('unknown command');
    expect(failure.stderr).toContain('KPM 9999.0.0 is available');
    freshCache();
    const toolVersion = invoke(['--version']);
    expect(toolVersion.stdout.trim()).toBe(version);
    expect(toolVersion.stderr).toBe('');
  } finally { rmSync(directory, { recursive: true, force: true }); }
});

test('notifier filesystem failures cannot fail the command', () => {
  const directory = mkdtempSync(join(tmpdir(), 'kpm-notice-failure-'));
  try {
    const preload = join(directory, 'tty.cjs');
    writeFileSync(preload, "Object.defineProperty(process.stdout, 'isTTY', {value: true}); Object.defineProperty(process.stderr, 'isTTY', {value: true});");
    const blockedCache = join(directory, 'file');
    writeFileSync(blockedCache, 'not a directory');
    writeFileSync(join(directory, 'package.yml'), 'version: 1.2.3\n');
    const result = spawnSync('node', ['--require', preload, launcher, 'version'], {
      cwd: directory, encoding: 'utf8',
      env: { ...process.env, XDG_CACHE_HOME: blockedCache, CI: '', KPM_NO_UPDATE_NOTIFIER: '', NO_UPDATE_NOTIFIER: '' },
    });
    expect(result.status).toBe(0);
    expect(result.stdout).toBe('1.2.3\n');
    expect(result.stderr).toBe('');
  } finally { rmSync(directory, { recursive: true, force: true }); }
});

test('a stalled detached update check cannot hold the command open', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'kpm-detached-notice-'));
  try {
    const preload = join(directory, 'stalled-registry.cjs');
    const completed = join(directory, 'worker-finished');
    writeFileSync(preload, `
      Object.defineProperty(process.stdout, 'isTTY', {value: true});
      Object.defineProperty(process.stderr, 'isTTY', {value: true});
      if (process.argv.includes('--check')) {
        require('node:https').get = () => {
          const request = new (require('node:events').EventEmitter)();
          request.destroy = () => require('node:fs').writeFileSync(process.env.KPM_TEST_WORKER_FINISHED, 'deadline reached');
          return request;
        };
      }
    `);
    writeFileSync(join(directory, 'package.yml'), 'version: 1.2.3\n');
    const started = Date.now();
    const result = spawnSync('node', [launcher, 'version'], {
      cwd: directory, encoding: 'utf8', timeout: 1500,
      env: {
        ...process.env, NODE_OPTIONS: `--require ${JSON.stringify(preload)}`,
        XDG_CACHE_HOME: directory, CI: '', KPM_NO_UPDATE_NOTIFIER: '', NO_UPDATE_NOTIFIER: '',
        npm_config_update_notifier: 'true', KPM_TEST_WORKER_FINISHED: completed,
      },
    });
    expect(result.status).toBe(0);
    expect(result.stdout).toBe('1.2.3\n');
    expect(result.stderr).toBe('');
    expect(Date.now() - started).toBeLessThan(1500);
    // The foreground has already exited while the worker's deadline is pending.
    expect(existsSync(completed)).toBe(false);
    const deadline = Date.now() + 4000;
    while (!existsSync(completed) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 25));
    expect(existsSync(completed)).toBe(true);
  } finally { rmSync(directory, { recursive: true, force: true }); }
});
