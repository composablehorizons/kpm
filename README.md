# KPM — Kotlin Package Manager

Version and publish Kotlin libraries with npm-style commands.

```sh
kpm version patch
kpm publish
```

Keep your library's version, Maven coordinates, and publishing metadata in one `package.yml`. KPM creates version commits and Git tags, and its Gradle plugin configures Maven Central publishing and signing for you.

KPM is for maintainers of Kotlin/JVM and Kotlin Multiplatform libraries. It works on macOS, Linux, and Windows, with x64 and ARM64 binaries included.

## Quick start

### 1. Download via npm

```sh
npm install --global @composablehorizons/kpm
```

### 2. Configure plugin repositories

In `settings.gradle.kts`:

```kotlin
pluginManagement {
    repositories {
        gradlePluginPortal()
    }
}
```

### 3. Add the Gradle plugin

In `build.gradle.kts`:

```kotlin
plugins {
    id("com.composables.kpm") version "0.1.0"
}
```

### 4. Add package metadata

Create `package.yml` next to `build.gradle.kts` and replace the example values:

```yaml
name: My Kotlin Library
description: Reusable Kotlin tools for my apps.
groupId: com.example
artifactId: my-library
version: 0.1.0-SNAPSHOT
url: https://github.com/example/my-library
licenses:
  - name: MIT License
    url: https://github.com/example/my-library/blob/main/LICENSE
developers:
  - id: example
    name: Example Developer
    email: developer@example.com
scm:
  connection: scm:git:https://github.com/example/my-library.git
  developerConnection: scm:git:ssh://git@github.com/example/my-library.git
  url: https://github.com/example/my-library
```

### 5. Verify setup

Run from the directory containing `package.yml`:

```sh
kpm version
kpm publish --dry-run
```

## Requirements

- npm to install the CLI.
- A Kotlin/JVM or Kotlin Multiplatform library with a Gradle wrapper and JDK 17+.
- Git for version commits and tags.
- For Maven Central publishing: a registered namespace, a Central Portal user token, and a GPG signing key.

## Release from your terminal

### Configure publishing credentials

Before publishing, you'll need a [Maven Central account and registered namespace](https://central.sonatype.org/publish/requirements/), a Central Portal user token, and a GPG signing key.

KPM uses the standard Gradle publishing properties. Set them through environment variables, your user-level Gradle configuration, or CI secrets. For example:

```sh
export ORG_GRADLE_PROJECT_mavenCentralUsername="YOUR_CENTRAL_TOKEN_USERNAME"
export ORG_GRADLE_PROJECT_mavenCentralPassword="YOUR_CENTRAL_TOKEN_PASSWORD"
export ORG_GRADLE_PROJECT_signingInMemoryKey="$(cat /path/to/private-signing-key.asc)"
export ORG_GRADLE_PROJECT_signingInMemoryKeyId="YOUR_KEY_ID"
export ORG_GRADLE_PROJECT_signingInMemoryKeyPassword="YOUR_KEY_PASSWORD"
```

The signing key must be ASCII-armored. The key ID and password depend on your key configuration. Keep credentials out of `package.yml` and version control.

### Create and publish a version

Start with a clean Git working tree:

```sh
./gradlew check
kpm version patch
kpm publish
```

With the quick-start metadata, `kpm version patch` changes `0.1.0-SNAPSHOT` to `0.1.0`, commits `package.yml`, and creates the annotated tag `v0.1.0`.

`kpm publish` builds the configured publications, signs them when signing credentials are provided, and uploads and automatically releases them on Maven Central. It uses your project's Gradle wrapper. It does not run your test suite or push Git changes; those are separate steps you control.

For later releases, use `patch`, `minor`, `major`, or an exact version:

```sh
kpm version minor
kpm version 1.0.0
```

## Release with GitHub Actions

Install a pinned KPM version in your workflow.

This workflow publishes when you push a `v*` tag. It verifies that the tag matches `package.yml`, runs your checks, then publishes from that tagged commit:

```yaml
name: Release
on:
  push:
    tags: ['v*']

permissions:
  contents: read

concurrency:
  group: release
  cancel-in-progress: false

jobs:
  publish:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-java@v5
        with:
          distribution: temurin
          java-version: '17'
      - uses: actions/setup-node@v5
        with:
          node-version: '22'
      - uses: gradle/actions/setup-gradle@v5
      - name: Install KPM
        run: npm install --global @composablehorizons/kpm@0.1.0
      - name: Verify release version
        run: test "v$(kpm version)" = "$GITHUB_REF_NAME"
      - run: ./gradlew check
      - name: Publish to Maven Central
        env:
          ORG_GRADLE_PROJECT_mavenCentralUsername: ${{ secrets.OSSRH_USERNAME }}
          ORG_GRADLE_PROJECT_mavenCentralPassword: ${{ secrets.OSSRH_PASSWORD }}
          ORG_GRADLE_PROJECT_signingInMemoryKey: ${{ secrets.SIGNING_KEY }}
          ORG_GRADLE_PROJECT_signingInMemoryKeyId: ${{ secrets.SIGNING_KEY_ID }}
          ORG_GRADLE_PROJECT_signingInMemoryKeyPassword: ${{ secrets.SIGNING_PASSWORD }}
        run: kpm publish
```

Add the corresponding secrets to your repository or organization. The example uses macOS to accommodate iOS targets; choose a runner and any additional SDK setup that match your library's targets.

With this workflow in place, create the version locally and push the commit and tag. GitHub Actions handles publication:

```sh
kpm version patch
git push --follow-tags
```

## Command reference

| Command | What it does |
| --- | --- |
| `kpm --version` | Prints the installed KPM version. |
| `kpm version` | Prints the current library version. |
| `kpm version patch` | Increments the patch version, or removes its prerelease suffix. |
| `kpm version minor` | Selects the next minor release. |
| `kpm version major` | Selects the next major release. |
| `kpm version 1.2.3` | Sets an exact semantic version. |
| `kpm version from-git` | Uses the nearest reachable Git tag, with the configured prefix removed. |
| `kpm publish --dry-run` | Shows the publishing task graph without executing it. |
| `kpm publish` | Publishes the current version through Gradle. |

Version commands accept these options before or after the version argument:

| Option | Use it to… |
| --- | --- |
| `--no-git-tag-version` | Update `package.yml` without committing or tagging. |
| `--message 'Release %s'` | Customize the commit and tag message; `%s` becomes the version. |
| `--tag-version-prefix release-` | Change the default `v` tag prefix. |
| `--allow-same-version` | Explicitly permit keeping the current version. |

Version changes require a clean working tree unless you use `--no-git-tag-version`. KPM checks for an existing tag before updating metadata. Version commands never push to a remote.

## package.yml reference

This example includes every supported field:

```yaml
name: My Kotlin Library
description: Reusable Kotlin tools for my apps.
groupId: com.example
artifactId: my-library
version: 0.1.0
url: https://github.com/example/my-library

licenses:
  - name: MIT License
    url: https://github.com/example/my-library/blob/main/LICENSE
    distribution: repo # Optional

developers:
  - id: example
    name: Example Developer
    email: developer@example.com # Optional
    url: https://github.com/example # Optional

scm:
  connection: scm:git:https://github.com/example/my-library.git
  developerConnection: scm:git:ssh://git@github.com/example/my-library.git
  url: https://github.com/example/my-library

issueManagement: # Optional; include both fields when present
  system: GitHub Issues
  url: https://github.com/example/my-library/issues
```

### Fields

| Field | Required | Purpose |
| --- | --- | --- |
| `name` | Yes | Display name of the library. |
| `description` | Yes | Summary of what the library does. |
| `groupId` | Yes | Maven namespace, such as `com.example`. Also sets the Gradle project group. |
| `artifactId` | Yes | Maven artifact name, such as `my-library`. |
| `version` | Yes | Package version in semantic version format, such as `1.2.3` or `1.2.3-SNAPSHOT`. Also sets the Gradle project version. |
| `url` | Yes | Library homepage or repository URL. |
| `licenses` | Yes | A list containing at least one license. |
| `licenses[].name` | Yes | License name. |
| `licenses[].url` | Yes | URL of the license text. |
| `licenses[].distribution` | No | Maven license distribution value, commonly `repo`. |
| `developers` | Yes | A list containing at least one developer. |
| `developers[].id` | Yes | Developer identifier, such as a GitHub username. |
| `developers[].name` | Yes | Developer or organization display name. |
| `developers[].email` | No | Developer contact email. |
| `developers[].url` | No | Developer website or profile URL. |
| `scm` | Yes | Source repository details. |
| `scm.connection` | Yes | Read-only SCM connection, such as `scm:git:https://github.com/example/my-library.git`. |
| `scm.developerConnection` | Yes | Developer SCM connection, such as `scm:git:ssh://git@github.com/example/my-library.git`. |
| `scm.url` | Yes | Browser URL for the source repository. |
| `issueManagement` | No | Issue tracker details. |
| `issueManagement.system` | When `issueManagement` is present | Tracker name, such as `GitHub Issues`. |
| `issueManagement.url` | When `issueManagement` is present | URL of the issue tracker. |

All text fields must be non-empty strings when supplied. Add multiple entries under `licenses` or `developers` as needed. Publishing metadata is written to the generated Maven POM.

### File location

Run the CLI from the directory containing `package.yml`. It reads only that directory and reports an error if the file is missing. The Gradle plugin reads it from the project where the plugin is applied. For publishing, that directory must also contain the Gradle wrapper.

Use KPM per library build. Keep dependency declarations and their versions in Gradle.
