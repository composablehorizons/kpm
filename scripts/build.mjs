import { mkdirSync, readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('../', import.meta.url));
const { version } = JSON.parse(readFileSync(new URL('../package.json', import.meta.url)));
for (const os of ['darwin', 'linux', 'windows']) {
  for (const arch of ['amd64', 'arm64']) {
    const directory = `${root}dist/${os}-${arch}`;
    mkdirSync(directory, { recursive: true });
    const result = spawnSync('go', ['build', '-trimpath', '-ldflags', `-s -w -X github.com/composablehorizons/kpm/internal/cli.binaryVersion=${version}`, '-o', `${directory}/kpm${os === 'windows' ? '.exe' : ''}`, '.'], {
      cwd: root, stdio: 'inherit', env: { ...process.env, GOOS: os, GOARCH: arch, CGO_ENABLED: '0' },
    });
    if (result.status !== 0) process.exit(result.status ?? 1);
    console.log(`Built ${os}/${arch}`);
  }
}
