import { test, expect } from 'bun:test';
import { version } from '../package.json';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
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
