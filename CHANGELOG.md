# Changelog

## [Unreleased]

### Changed

- Publish the npm CLI, Maven plugin, and GitHub release through the GitHub Actions release workflow. npm uses trusted publishing; manual recovery can target one registry without repeating an accepted Maven upload.

## [0.2.1] - 2026-10-22

### Added

- A daily background update check in the npm launcher, with cached terminal notices and an npm upgrade command. Update checks never wait on the network in the foreground or affect command exit status, and can be disabled with `KPM_NO_UPDATE_NOTIFIER=1`.

## [0.2.0] - 2026-10-22

### Added

- npm-style dependency commands: `install` (`i`), `uninstall` (`remove`, `rm`), `list` (`ls`), `outdated`, and `update` (`up`).
- `--target`, `--api`, and `--implementation` options, with shorthand and scoped dependency declarations in `package.yml`.
- Gradle dependency resolution, version lookup, and enabled dependency-lock updates using project repositories and credentials.
- An optional nested `publishing` block. Packages without publishing metadata can use KPM for dependency management.

### Deprecated

- Top-level publishing fields remain supported with a warning and will be removed in 1.0.0. Move them under `publishing`; package name, description, and version stay at the top level.

## [0.1.1] - 2026-10-21

### Changed

- Return after uploading a release instead of waiting for Maven Central validation. Automatic release stays enabled; artifacts become downloadable after Maven Central finishes processing them.
