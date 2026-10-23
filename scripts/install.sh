#!/bin/sh
set -eu

# Keep execution inside a function so a truncated curl | sh cannot half-install.
kpm_install() {
    version=${KPM_VERSION:-}
    install_dir=${KPM_INSTALL_DIR:-"$HOME/.local/bin"}
    release_base=${KPM_RELEASE_BASE_URL:-https://github.com/composablehorizons/kpm/releases}
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --version|--bin-dir)
                [ "$#" -ge 2 ] || { printf 'kpm: %s requires a value\n' "$1" >&2; return 1; }
                case "$1" in --version) version=$2 ;; --bin-dir) install_dir=$2 ;; esac
                shift 2 ;;
            --help|-h)
                printf 'Usage: sh install.sh [--version VERSION] [--bin-dir DIRECTORY]\n'
                printf 'Defaults: latest published stable release, ~/.local/bin\n'
                return 0 ;;
            *) printf 'kpm: unknown installer option: %s\n' "$1" >&2; return 1 ;;
        esac
    done
    case "$(uname -s)" in
        Darwin) platform=darwin ;;
        Linux) platform=linux ;;
        *) printf 'kpm: use scripts/install.ps1 on Windows; this installer supports macOS and Linux\n' >&2; return 1 ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) printf 'kpm: unsupported architecture; available binaries are x64 and ARM64\n' >&2; return 1 ;;
    esac
    for tool in curl tar awk grep mktemp; do
        command -v "$tool" >/dev/null 2>&1 || { printf 'kpm: install %s and try again\n' "$tool" >&2; return 1; }
    done
    if command -v sha256sum >/dev/null 2>&1; then
        checksum_tool=sha256sum
    elif command -v shasum >/dev/null 2>&1; then
        checksum_tool=shasum
    else
        printf 'kpm: SHA-256 verification requires sha256sum or shasum\n' >&2; return 1
    fi
    if [ -z "$version" ]; then
        release_url=$(curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 --head --output /dev/null --write-out '%{url_effective}' "$release_base/latest") || {
            printf 'kpm: could not find the latest published release; retry or use --version VERSION\n' >&2; return 1;
        }
        version=${release_url##*/}
    fi
    version=${version#v}
    printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
        printf 'kpm: invalid release version: %s\n' "$version" >&2; return 1;
    }
    [ -n "$install_dir" ] || { printf 'kpm: the installation directory must not be empty\n' >&2; return 1; }
    work_dir=$(mktemp -d)
    staged_binary=''
    trap 'rm -rf "$work_dir"; if [ -n "$staged_binary" ]; then rm -f "$staged_binary"; fi' 0
    trap 'exit 1' 1 2 15
    asset="kpm_${version}_${platform}_${arch}.tar.gz"
    download="$release_base/download/v$version"
    printf 'Downloading KPM %s for %s/%s...\n' "$version" "$platform" "$arch"
    curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 "$download/$asset" -o "$work_dir/$asset" || {
        printf 'kpm: download failed; check that v%s is published with native release assets\n' "$version" >&2; return 1;
    }
    curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 "$download/SHA256SUMS" -o "$work_dir/SHA256SUMS" || return 1
    expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "$work_dir/SHA256SUMS")
    [ "${#expected}" -eq 64 ] && printf '%s\n' "$expected" | grep -Eq '^[0-9a-f]{64}$' || {
        printf 'kpm: missing or invalid checksum for %s\n' "$asset" >&2; return 1;
    }
    if [ "$checksum_tool" = sha256sum ]; then
        actual=$(sha256sum "$work_dir/$asset" | awk '{ print $1 }')
    else
        actual=$(shasum -a 256 "$work_dir/$asset" | awk '{ print $1 }')
    fi
    [ "$actual" = "$expected" ] || { printf 'kpm: checksum mismatch; existing installation was not changed\n' >&2; return 1; }
    tar -xzf "$work_dir/$asset" -C "$work_dir" kpm
    chmod 755 "$work_dir/kpm"
    installed_version=$("$work_dir/kpm" --version) || { printf 'kpm: downloaded binary cannot run on this machine\n' >&2; return 1; }
    [ "$installed_version" = "$version" ] || { printf 'kpm: downloaded binary version does not match the release\n' >&2; return 1; }
    mkdir -p "$install_dir"
    staged_binary=$(mktemp "$install_dir/.kpm-install.XXXXXX")
    cp "$work_dir/kpm" "$staged_binary"
    chmod 755 "$staged_binary"
    mv -f "$staged_binary" "$install_dir/kpm"
    staged_binary=''
    printf 'Installed KPM %s to %s/kpm\n' "$version" "$install_dir"
    case ":${PATH:-}:" in
        *":$install_dir:"*) printf 'Run: kpm --version\n' ;;
        *) printf 'Add this directory to PATH: %s\n' "$install_dir"
           if [ "$install_dir" = "$HOME/.local/bin" ]; then printf 'For this shell: export PATH="$HOME/.local/bin:$PATH"\n'; fi ;;
    esac
}

kpm_install "$@"
