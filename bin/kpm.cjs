#!/usr/bin/env node
const { spawnSync } = require('node:child_process');
const { join } = require('node:path');
const os = { darwin: 'darwin', linux: 'linux', win32: 'windows' }[process.platform];
if (!os || !['arm64', 'x64'].includes(process.arch)) {
  console.error(`kpm: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}
const arch = process.arch === 'x64' ? 'amd64' : 'arm64';
let updateNotice = '';
// The notifier is optional and must never affect native command execution.
try {
  if (!process.argv.slice(2).includes('--version')) {
    const { prepareUpdateNotification } = require('./update-notifier.cjs');
    updateNotice = prepareUpdateNotification({ currentVersion: require('../package.json').version });
  }
} catch {}
const executable = join(__dirname, '..', 'dist', `${os}-${arch}`, os === 'windows' ? 'kpm.exe' : 'kpm');
const result = spawnSync(executable, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
  console.error(`kpm: cannot start bundled binary: ${result.error.message}`);
  process.exit(1);
}
if (updateNotice) { try { process.stderr.write(updateNotice); } catch {} }
if (result.signal) process.kill(process.pid, result.signal);
else process.exit(result.status ?? 1);
