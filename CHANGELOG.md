# Changelog

## [Unreleased]

### Changed

- Distribute native CLI archives and SHA-256 checksums through GitHub Releases. Install using `scripts/install.sh` on macOS/Linux or `scripts/install.ps1` on Windows; rerun the installer to update.
- Remove the npm package, Node launcher, npm update notifier, and Bun build tooling. Build release archives with Go and use `VERSION` as the shared CLI/Gradle plugin release version.

## [0.4.0] - 2026-10-23

### Added

- `kpm init` configures KPM in an existing Gradle project: creates `package.yml`, applies the Gradle plugin, and adds missing repositories. Use `-y` / `--yes` to accept inferred defaults. Existing manifest values and dependencies are preserved; unsupported layouts receive manual setup instructions without partial edits.
- Dependency and publishing commands find a parent Gradle wrapper and run only for the current package, including modules with custom Gradle project directory mappings.

### Changed

- Descriptions are optional for dependency management and remain required for publishing.
- The README quick start uses `kpm init`; maintainer release instructions now live in `RELEASING.md`.
- The release workflow creates a draft GitHub release for maintainer review and later publication, after publishing to npm and uploading to Maven Central.

### Fixed

- Use npm’s registry retries when checking publication state instead of a single 15-second request. Treat only an explicit `E404` as an unpublished version, so releases can safely resume after temporary registry failures.

## [0.3.0] - 2026-10-22

### Changed

- Bare `group:artifact` installs and manifest entries now defer version selection to Gradle constraints. Use `kpm i group:artifact@latest` to look up and save an exact latest release. Versionless declarations are preserved by `update --save`.

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
