param(
    [string]$Version = $env:KPM_VERSION,
    [string]$InstallDir = $env:KPM_INSTALL_DIR,
    [switch]$NoPath
)

$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'Use scripts/install.sh on macOS and Linux.' }
if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\kpm\bin' }
$releaseBase = 'https://github.com/composablehorizons/kpm/releases'
if ($env:KPM_RELEASE_BASE_URL) { $releaseBase = $env:KPM_RELEASE_BASE_URL.TrimEnd('/') }
$architecture = $env:PROCESSOR_ARCHITEW6432
if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
switch ($architecture) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { throw 'Unsupported architecture. KPM provides Windows x64 and ARM64 binaries.' }
}
# Windows PowerShell 5.1 may otherwise default to older TLS versions.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
if (-not $Version) {
    $response = Invoke-WebRequest -Uri "$releaseBase/latest" -Method Head -UseBasicParsing -TimeoutSec 120
    if ($PSVersionTable.PSVersion.Major -ge 6) {
        $releaseUri = $response.BaseResponse.RequestMessage.RequestUri
    } else {
        $releaseUri = $response.BaseResponse.ResponseUri
    }
    $Version = $releaseUri.AbsolutePath.TrimEnd('/').Split('/')[-1]
}
$Version = $Version -creplace '^v', ''
if ($Version -cnotmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$') {
    throw "Invalid release version: $Version"
}
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('kpm-install-' + [Guid]::NewGuid().ToString('N'))
$stagedBinary = $null
try {
    New-Item -ItemType Directory -Path $temporary | Out-Null
    $asset = "kpm_${Version}_windows_${arch}.zip"
    $archive = Join-Path $temporary $asset
    $checksums = Join-Path $temporary 'SHA256SUMS'
    $download = "$releaseBase/download/v$Version"
    Write-Host "Downloading KPM $Version for windows/$arch..."
    Invoke-WebRequest -Uri "$download/$asset" -OutFile $archive -UseBasicParsing -TimeoutSec 120
    Invoke-WebRequest -Uri "$download/SHA256SUMS" -OutFile $checksums -UseBasicParsing -TimeoutSec 120
    $pattern = '^([0-9a-f]{64})  ' + [regex]::Escape($asset) + '$'
    $entries = @(Get-Content $checksums | Where-Object { $_ -cmatch $pattern })
    if ($entries.Count -ne 1) { throw "Missing or invalid checksum for $asset" }
    $expected = [regex]::Match($entries[0], $pattern).Groups[1].Value
    $actual = (Get-FileHash -Path $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -cne $expected) { throw 'Checksum mismatch. Existing installation was not changed.' }
    Expand-Archive -Path $archive -DestinationPath $temporary
    $binary = Join-Path $temporary 'kpm.exe'
    $installedVersion = & $binary --version
    if ($LASTEXITCODE -ne 0 -or $installedVersion -cne $Version) { throw 'Downloaded binary version does not match the release.' }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $stagedBinary = Join-Path $InstallDir ('.kpm-install-' + [Guid]::NewGuid().ToString('N') + '.exe')
    Copy-Item -LiteralPath $binary -Destination $stagedBinary
    Move-Item -LiteralPath $stagedBinary -Destination (Join-Path $InstallDir 'kpm.exe') -Force
    $stagedBinary = $null
    if (-not $NoPath) {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $paths = @($userPath -split ';' | Where-Object { $_ })
        if ($paths -notcontains $InstallDir) {
            [Environment]::SetEnvironmentVariable('Path', (($paths + $InstallDir) -join ';'), 'User')
        }
        if (($env:Path -split ';') -notcontains $InstallDir) { $env:Path = "$InstallDir;$env:Path" }
    }
    Write-Host "Installed KPM $Version to $InstallDir\kpm.exe"
    if ($NoPath) { Write-Host "Add this directory to PATH: $InstallDir" }
    else { Write-Host 'Open a new terminal, then run: kpm --version' }
} finally {
    if ($stagedBinary -and (Test-Path -LiteralPath $stagedBinary)) { Remove-Item -LiteralPath $stagedBinary -Force }
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Recurse -Force }
}
