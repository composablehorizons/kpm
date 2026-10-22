const fs = require('node:fs');
const https = require('node:https');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');

const CHECK_INTERVAL = 24 * 60 * 60 * 1000;
const LOCK_LIFETIME = 30 * 1000;
const REGISTRY_URL = 'https://registry.npmjs.org/@composablehorizons%2fkpm/latest';

function versionParts(version) {
  if (typeof version !== 'string' || version.length > 128) return null;
  const match = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/.exec(version);
  return match ? match.slice(1, 4).map(BigInt) : null;
}

function newerVersion(latest, current) {
  const left = versionParts(latest);
  const right = versionParts(current);
  if (!left || !right) return false;
  for (let i = 0; i < left.length; i++) {
    if (left[i] !== right[i]) return left[i] > right[i];
  }
  return false;
}

function defaultCacheDirectory(env = process.env) {
  if (env.XDG_CACHE_HOME) return path.join(env.XDG_CACHE_HOME, 'kpm');
  if (process.platform === 'darwin') return path.join(os.homedir(), 'Library', 'Caches', 'kpm');
  if (process.platform === 'win32') return path.join(env.LOCALAPPDATA || os.tmpdir(), 'kpm', 'Cache');
  return path.join(os.homedir(), '.cache', 'kpm');
}

function readCache(directory) {
  try {
    const file = path.join(directory, 'update.json');
    if (fs.statSync(file).size > 16 * 1024) return {};
    const value = JSON.parse(fs.readFileSync(file, 'utf8'));
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  } catch { return {}; }
}

function writeCache(directory, value) {
  const file = path.join(directory, `update-${process.pid}.tmp`);
  try {
    fs.writeFileSync(file, JSON.stringify(value), { mode: 0o600 });
    fs.renameSync(file, path.join(directory, 'update.json'));
  } finally {
    try { fs.unlinkSync(file); } catch {}
  }
}

function recent(timestamp, now, interval) {
  return Number.isFinite(timestamp) && timestamp <= now && now - timestamp < interval;
}

function launchCheck(directory) {
  const child = spawn(process.execPath, [__filename, '--check', directory], {
    detached: true,
    stdio: 'ignore',
    windowsHide: true,
  });
  // A missing executable or denied subprocess must never become an uncaught error.
  child.on('error', () => {});
  child.unref();
}

// Only local cache reads and a detached spawn run in the foreground. Never wait
// for the worker: short-lived commands see its result on a later invocation.
function prepareUpdateNotification({
  currentVersion,
  interactive = !!process.stdout.isTTY && !!process.stderr.isTTY,
  env = process.env,
  now = Date.now(),
  cacheDirectory,
  startCheck = launchCheck,
} = {}) {
  try {
    if (!interactive || env.CI || env.KPM_NO_UPDATE_NOTIFIER || env.NO_UPDATE_NOTIFIER
        || env.npm_config_update_notifier === 'false' || !versionParts(currentVersion)) return '';
    const directory = cacheDirectory || defaultCacheDirectory(env);
    let cache = readCache(directory);
    let notice = '';
    if (newerVersion(cache.latest, currentVersion)
        && (cache.notifiedVersion !== cache.latest || !recent(cache.notifiedAt, now, CHECK_INTERVAL))) {
      notice = `\nKPM ${cache.latest} is available (installed: ${currentVersion}).\nUpdate: npm install -g @composablehorizons/kpm@latest\n`;
      cache = { ...cache, notifiedVersion: cache.latest, notifiedAt: now };
      // Failure to write a notification timestamp should not hide the notice.
      try { writeCache(directory, cache); } catch {}
    }
    if (!recent(cache.checkedAt, now, CHECK_INTERVAL)) {
      fs.mkdirSync(directory, { recursive: true });
      const lock = path.join(directory, 'update.lock');
      try {
        if (!recent(fs.statSync(lock).mtimeMs, now, LOCK_LIFETIME)) fs.unlinkSync(lock);
      } catch {}
      let fd;
      try { fd = fs.openSync(lock, 'wx', 0o600); } catch { return notice; }
      fs.closeSync(fd);
      try {
        // Reserve the daily attempt even on offline runs or a failed spawn.
        writeCache(directory, { ...readCache(directory), ...cache, checkedAt: now });
        startCheck(directory);
      } catch {
        try { fs.unlinkSync(lock); } catch {}
      }
    }
    return notice;
  } catch { return ''; }
}

function fetchLatestVersion({ request = https.get, url = REGISTRY_URL, timeoutMs = 2000 } = {}) {
  return new Promise(resolve => {
    let requestHandle;
    let responseHandle;
    let done = false;
    const finish = latest => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      responseHandle?.destroy();
      requestHandle?.destroy();
      resolve(versionParts(latest) ? latest : null);
    };
    // A wall-clock deadline also covers DNS, TLS, and servers trickling data.
    const timer = setTimeout(() => finish(null), timeoutMs);
    try {
      requestHandle = request(url, { headers: { Accept: 'application/json' } }, response => {
        responseHandle = response;
        response.on('error', () => finish(null));
        if (response.statusCode !== 200) { finish(null); return; }
        let data = '';
        let bytes = 0;
        response.setEncoding('utf8');
        response.on('data', chunk => {
          bytes += Buffer.byteLength(chunk);
          if (bytes > 64 * 1024) { finish(null); return; }
          data += chunk;
        });
        response.on('end', () => {
          try { finish(JSON.parse(data).version); } catch { finish(null); }
        });
      });
      requestHandle.on('error', () => finish(null));
    } catch { finish(null); }
  });
}

async function checkForUpdate(directory) {
  try {
    const latest = await fetchLatestVersion();
    // Preserve notification timestamps written by concurrent invocations.
    const cache = readCache(directory);
    writeCache(directory, { ...cache, checkedAt: Date.now(), ...(latest ? { latest } : {}) });
  } catch {
    // Update checks must never affect the command, including filesystem errors.
  } finally {
    try { fs.unlinkSync(path.join(directory, 'update.lock')); } catch {}
  }
}

module.exports = { CHECK_INTERVAL, prepareUpdateNotification, fetchLatestVersion, newerVersion };
if (require.main === module && process.argv[2] === '--check' && process.argv[3]) {
  checkForUpdate(process.argv[3]).catch(() => {});
}
