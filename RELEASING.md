# Releasing KPM

Production publishing runs in [the Release workflow](.github/workflows/release.yml).
It builds native CLI archives, publishes the project and settings Gradle plugins and their markers to Maven
Central, and creates a draft GitHub release with the binaries, installers, and
`SHA256SUMS` attached. Publish the draft after reviewing it to make those assets
available to the installers. CLI distribution uses GitHub Releases.

## Release steps

1. Start on `main` with a clean working tree, pull the latest changes, and choose
   the next semantic version. Use a patch for fixes, a minor for new features,
   and a major for breaking changes; before 1.0, use a minor for breaking changes.

2. Update the root `VERSION` file. For example:

   ```sh
   release_version=0.5.0
   printf '%s\n' "$release_version" > VERSION
   ```

   Both the Go archive builder and the Gradle plugin read this file. Also update
   the development fallback plugin version in `internal/cli/init.go`, its expected
   version in `internal/cli/init_test.go`, and current-version README examples.

3. Update `CHANGELOG.md`. Collect all user-visible changes under `[Unreleased]`,
   including new features, fixes, and breaking changes. Move those entries into
   a `## [<version>] - YYYY-MM-DD` section for this release, and leave an empty
   `[Unreleased]` section above it. Review the entries against the commits since
   the previous release tag.

4. Build and verify the release:

   ```sh
   go run ./scripts/build.go
   go test ./...
   ./gradlew :gradle-plugin:test :gradle-plugin:validatePlugins :gradle-settings-plugin:validatePlugins --console=plain
   KPM_GRADLE_INTEGRATION=1 go test ./internal/cli -run TestInitGradleIntegration -count=1
   ./gradlew -p examples/kotlin-library compileKotlinJvm compileKotlinWasmJs generatePomFileForKotlinMultiplatformPublication generatePomFileForJvmPublication --console=plain
   ```

   `dist/` contains six archives: `.tar.gz` for macOS/Linux and `.zip` for
   Windows, each for x64 and ARM64. It also contains the installers and a
   `SHA256SUMS` manifest. The build injects the version into each executable.
   CI tests the shell installer on macOS/Linux and PowerShell on Windows.

5. Review the diff, commit the release files, and push the commit. Wait for its
   Verify workflow to pass before tagging:

   ```sh
   git diff --check
   git diff
   git add VERSION CHANGELOG.md README.md internal/cli/init.go internal/cli/init_test.go
   git commit -m "Release $release_version"
   git push origin main
   ```

6. Create an annotated tag at the verified release commit and push it to start
   the Release workflow. The tag must match `VERSION` exactly:

   ```sh
   git tag -a "v$release_version" -m "Release $release_version"
   git push origin "v$release_version"
   ```

7. Wait for the workflow to finish. It checks the tagged source, verifies all
   archive checksums, and uploads those exact assets to a draft GitHub release.
   Confirm the Maven upload succeeded; Central may still be processing an
   accepted upload before artifacts become downloadable.

8. Open the **draft** on GitHub Releases. Review and edit the generated notes
   against `CHANGELOG.md`, check the version, prerelease status, and all nine
   assets (six archives, two installers, and `SHA256SUMS`). Click **Publish release**
   to make the release and its downloads public. After reviewing, you can also:

   ```sh
   gh release edit "v$release_version" --draft=false
   ```

   Stable releases are selected by the installer's latest-release URL. Prereleases
   must be installed by explicitly selecting their version. The draft holds back
   the CLI downloads; the Maven upload has already happened at this point.

9. Verify the public installation:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/composablehorizons/kpm/main/scripts/install.sh | KPM_VERSION="$release_version" sh
   "$HOME/.local/bin/kpm" --version
   ```

   Verify the Windows installer in PowerShell as well. Re-running an installer
   updates or replaces the installed CLI after verifying its download.

## Publishing credentials

The release workflow uses its GitHub token with `contents: write` to upload release
assets. Maven publishing uses the existing `OSSRH_USERNAME`, `OSSRH_PASSWORD`,
`SIGNING_KEY`, `SIGNING_KEY_ID`, and `SIGNING_PASSWORD` repository secrets. Production
Maven uploads run in CI through the project's Gradle wrapper.

## Recover a partial release

Dispatch the current workflow from `main` with the existing release tag. Set
`publish_maven=false` if Central already accepted that version, even when it is
not yet downloadable:

```sh
gh workflow run release.yml --ref main \
  -f tag=v0.5.0 -f publish_maven=false
```

The workflow keeps the publication state of an existing GitHub release and uploads
missing assets without replacing existing files. Existing files must match the
verified artifact; investigate differences rather than overwriting released
binaries. Do not move an existing release tag or repeat an accepted Maven upload.

Releases before this distribution migration do not contain `VERSION` or the Go
archive builder. To add native downloads to an older release without changing its
CLI version, recover the original binaries from that release's successful Actions
build, then package them using the current builder:

```sh
go run ./scripts/build.go --binaries-dir /path/to/original/dist --output /path/to/assets
```

The current `VERSION` must match those original binaries. Verify the native
executable version and checksums before attaching the resulting assets to the
existing mutable release. This packages the original executable bytes without
rebuilding or retagging that version.
