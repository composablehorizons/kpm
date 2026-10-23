package cli

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Opt in because this exercises a real Gradle build and needs a JDK and plugin
// dependencies. CI runs it alongside the Gradle plugin's own integration tests.
func TestInitGradleIntegration(t *testing.T) {
	if os.Getenv("KPM_GRADLE_INTEGRATION") != "1" {
		t.Skip("set KPM_GRADLE_INTEGRATION=1 to exercise a real Gradle build")
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper fixture; CLI unit tests also run on Windows")
	}
	for _, dsl := range []string{"kotlin", "groovy"} {
		t.Run(dsl, func(t *testing.T) { testInitGradleConsumer(t, dsl == "groovy") })
	}
}

func testInitGradleConsumer(t *testing.T, groovy bool) {
	suffix := ".gradle.kts"
	if groovy {
		suffix = ".gradle"
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repository, err := filepath.Abs(filepath.Join(cwd, "../.."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// The real wrapper stays in the repository; this wrapper delegates while
	// retaining the temporary consumer's working directory.
	wrapper := "#!/bin/sh\nexec '" + strings.ReplaceAll(filepath.Join(repository, "gradlew"), "'", "'\\''") + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte(wrapper), 0755); err != nil {
		t.Fatal(err)
	}
	settings := `pluginManagement {
    includeBuild("` + filepath.ToSlash(repository) + `")
    repositories { gradlePluginPortal(); mavenCentral() }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories { maven { url = uri("repo") } }
}
rootProject.name = "consumer"
include(":app", ":app:child", ":other")
project(":app").projectDir = file("modules/mobile")
project(":app:child").projectDir = file("modules/mobile/child")
`
	writeFile(t, dir, "settings"+suffix, settings)
	writeFile(t, dir, "build"+suffix, `plugins { id("java-library") }`)
	module := filepath.Join(dir, "modules/mobile")
	child := filepath.Join(module, "child")
	other := filepath.Join(dir, "other")
	for _, path := range []string{module, child, other} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, module, "build"+suffix, "plugins { id(\"java-library\") }\nversion = \"2.3.4\"\n")
	for _, path := range []string{child, other} {
		writeFile(t, path, "build"+suffix, `plugins { id("java-library"); id("com.composables.kpm") }`)
		writeFile(t, path, "package.yml", "name: Untouched\nversion: 0.1.0\ndependencies: [example:missing:1.0.0]\n")
	}
	artifact := filepath.Join(dir, "repo/example/tool/1.0.0")
	if err := os.MkdirAll(artifact, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, artifact, "tool-1.0.0.pom", `<project><modelVersion>4.0.0</modelVersion><groupId>example</groupId><artifactId>tool</artifactId><version>1.0.0</version></project>`)
	jar, err := os.Create(filepath.Join(artifact, "tool-1.0.0.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if err := zip.NewWriter(jar).Close(); err != nil {
		t.Fatal(err)
	}
	if err := jar.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, module} {
		if output, err := invoke(path, "init", "-y"); err != nil {
			t.Fatalf("init: %v\n%s", err, output)
		}
	}
	rootManifest := readFile(t, dir, "package.yml")
	if output, err := invoke(module, "i", "example:tool:1.0.0"); err != nil {
		t.Fatalf("install: %v\n%s", err, output)
	}
	if rootManifest != readFile(t, dir, "package.yml") {
		t.Fatal("module install changed root manifest")
	}
	if output, err := invoke(module, "list"); err != nil || !strings.Contains(output, "example:tool") {
		t.Fatalf("module list: %v\n%s", err, output)
	}
	// Child/sibling packages have unresolvable dependencies; selecting them would
	// make either of these commands fail (and could overwrite the selected report).
	if output, err := invoke(dir, "list"); err != nil {
		t.Fatalf("root list selected other packages: %v\n%s", err, output)
	}
	if output, err := invoke(module, "version", "patch", "--no-git-tag-version"); err != nil {
		t.Fatalf("version: %v\n%s", err, output)
	}
	manifest := readFile(t, module, "package.yml")
	if !strings.Contains(manifest, "version: 2.3.5") {
		t.Fatal("did not preserve/bump existing module version")
	}
	// Publication remains opt-in. This checks the real scoped task graph only.
	writeFile(t, module, "package.yml", manifest+`description: Integration test library
publishing:
  groupId: example.library
  artifactId: tool
  url: https://example.test
  licenses:
    - name: MIT
      url: https://example.test/LICENSE
  developers:
    - id: example
      name: Example
  scm:
    connection: scm:git:https://example.test/repo.git
    developerConnection: scm:git:ssh://git@example.test/repo.git
    url: https://example.test/repo
`)
	if output, err := invoke(module, "publish", "--dry-run"); err != nil || !strings.Contains(output, ":app:publishAllPublicationsToMavenCentral") {
		t.Fatalf("module publish dry run: %v\n%s", err, output)
	}
}
