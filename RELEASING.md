# Releasing KPM

All production publishing runs in [the Release workflow](.github/workflows/release.yml): the npm CLI, the Maven plugin and marker, and a draft GitHub release. Local work prepares the version commit and `v<version>` tag; pushing that tag starts the workflow. Do not run `npm publish` or the Gradle production publishing task locally.

The workflow checks out the immutable tag, verifies it matches `package.json`, builds all six native binaries, runs CLI and Gradle checks, and verifies installation of the packed npm archive. It publishes that exact archive with npm trusted publishing and uploads the Maven publications using the existing signing/Central secrets. A draft GitHub release with generated notes is created after successful publication jobs. A maintainer reviews and publishes the draft separately. Prerelease npm versions use the `next` dist-tag; stable versions use `latest`.

## Release steps

1. Start on `main` with a clean working tree, pull the latest changes, and choose
   the next semantic version. Use a patch for fixes, a minor for new features,
   and a major for breaking changes; before 1.0, use a minor for breaking changes.

2. Update `package.json` without creating the commit or tag yet. For example:

   ```sh
   release_version=0.4.0
   npm version "$release_version" --no-git-tag-version --ignore-scripts
   ```

   The Gradle plugin and native CLI builds read their release version from
   `package.json`. Also update the development fallback plugin version in
   `internal/cli/init.go`, its expected version in `internal/cli/init_test.go`,
   and current-version examples in the README.

3. Update `CHANGELOG.md`. Collect all user-visible changes under `[Unreleased]`,
   including new features, fixes, and breaking changes. Move those entries into
   a `## [<version>] - YYYY-MM-DD` section for this release, and leave an empty
   `[Unreleased]` section above it for future changes. Review the entries against
   the commits since the previous release tag.

4. Build and verify the release:

   ```sh
   bun run build
   bun run test
   ./gradlew :gradle-plugin:test :gradle-plugin:validatePlugins --console=plain
   KPM_GRADLE_INTEGRATION=1 go test ./internal/cli -run TestInitGradleIntegration -count=1
   ./gradlew -p examples/kotlin-library compileKotlinJvm compileKotlinWasmJs generatePomFileForKotlinMultiplatformPublication generatePomFileForJvmPublication --console=plain
   ```

5. Review the diff, commit the release files, and push the commit. Wait for its
   Verify workflow to pass before tagging:

   ```sh
   git diff --check
   git diff
   git add package.json CHANGELOG.md README.md internal/cli/init.go internal/cli/init_test.go
   git commit -m "Release $release_version"
   git push origin main
   ```

6. Create an annotated tag at the verified release commit and push it to start
   the Release workflow. The tag must match `package.json` exactly:

   ```sh
   git tag -a "v$release_version" -m "Release $release_version"
   git push origin "v$release_version"
   ```

7. Wait for the Release workflow to finish. Confirm the npm version is available
   and the Maven upload succeeded. Maven Central may still be processing an
   accepted upload before the artifacts become downloadable:

   ```sh
   npm view "@composablehorizons/kpm@$release_version" version
   ```

8. Open the **draft** on the repository's GitHub Releases page. Review and edit
   the generated release notes against `CHANGELOG.md`, check the version and
   prerelease status, then click **Publish release** to make it public. After
   reviewing, you can also publish it from the terminal:

   ```sh
   gh release edit "v$release_version" --draft=false
   ```

   The draft only holds back the GitHub release announcement. npm publication
   and the Maven upload have already happened by this point.

## One-time publishing setup

One-time npm setup: authorize GitHub repository `composablehorizons/kpm`, workflow filename `release.yml`, to publish `@composablehorizons/kpm`. Use [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/) with direct `npm publish` allowed; the workflow has `id-token: write` and needs no `NPM_TOKEN` secret. A maintainer can configure this in npm package settings or with:

```sh
npm trust github @composablehorizons/kpm --repo composablehorizons/kpm --file release.yml --allow-publish --yes
```

This configuration step may require a one-time security-key approval. Subsequent GitHub releases authenticate through OIDC without local npm login or per-release authentication prompts.

## Recover a partial release

For recovery, dispatch the current workflow from `main` and select the existing release tag. Its `publish_npm` and `publish_maven` inputs let you retry one registry without repeating the other upload. The npm publication check uses `npm view` with two retries for temporary network/server failures and a 60-second request timeout. Only an explicit npm `E404` means the version is missing; other failures stop publication. Existing npm versions and GitHub releases are skipped. Maven Central processes accepted uploads asynchronously; disable `publish_maven` if that version was already accepted, even when it is not yet downloadable. Do not move a published tag.

For example, finish only the npm publication of `v0.2.1`:

```sh
gh workflow run release.yml --ref main \
  -f tag=v0.2.1 -f publish_npm=true -f publish_maven=false
```
