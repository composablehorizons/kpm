package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func initProject(t *testing.T, groovy bool) string {
	t.Helper()
	dir := t.TempDir()
	suffix := ".gradle.kts"
	plugin := `id("org.jetbrains.kotlin.jvm") version "2.4.0"`
	if groovy {
		suffix = ".gradle"
		plugin = `id 'org.jetbrains.kotlin.jvm' version '2.4.0'`
	}
	writeFile(t, dir, "settings"+suffix, "rootProject.name = \"hello-kotlin\"\n")
	writeFile(t, dir, "build"+suffix, "plugins { "+plugin+" }\nversion = \"1.2.3\" // retain this note\ndependencies { implementation(\"example:existing:1.0\") }\n")
	wrapper := "gradlew"
	if runtime.GOOS == "windows" {
		wrapper += ".bat"
	}
	writeFile(t, dir, wrapper, "")
	return dir
}

func TestInitSetsUpKotlinAndGroovyAndIsRepeatable(t *testing.T) {
	for _, groovy := range []bool{false, true} {
		dir := initProject(t, groovy)
		if _, err := invoke(dir, "init", "-y"); err != nil {
			t.Fatal(err)
		}
		p, err := readPackage(dir)
		if err != nil {
			t.Fatal(err)
		}
		if field(p.document.Content[0], "name").Value != "hello-kotlin" || p.version.Value != "1.2.3" {
			t.Fatal("lost inferred package metadata")
		}
		if field(p.document.Content[0], "description") != nil {
			t.Fatal("invented description")
		}
		suffix := ".gradle.kts"
		if groovy {
			suffix = ".gradle"
		}
		build := readFile(t, dir, "build"+suffix)
		settings := readFile(t, dir, "settings"+suffix)
		manifest := readFile(t, dir, "package.yml")
		if !strings.Contains(build, `id("com.composables.kpm") version "0.3.0"`) || !strings.Contains(build, "example:existing:1.0") || !strings.Contains(build, "retain this note") {
			t.Fatalf("incorrect build:\n%s", build)
		}
		if strings.Contains(build, `version = "1.2.3"`) {
			t.Fatal("Gradle would override manifest version")
		}
		if !strings.Contains(settings, "pluginManagement") || !strings.Contains(settings, "mavenCentral()") || !strings.Contains(build, "mavenCentral()") {
			t.Fatal("repositories not configured")
		}
		if _, err := invoke(dir, "init", "--yes"); err != nil {
			t.Fatal(err)
		}
		if build != readFile(t, dir, "build"+suffix) || settings != readFile(t, dir, "settings"+suffix) || manifest != readFile(t, dir, "package.yml") {
			t.Fatal("second init changed files")
		}
	}
}

func TestInitInteractiveAndCancellation(t *testing.T) {
	dir := initProject(t, false)
	var out bytes.Buffer
	if err := initCommand(dir, nil, strings.NewReader("My app\nbad\n2.0.0\nShared code\n"), &out, &out); err != nil {
		t.Fatal(err)
	}
	manifest := readFile(t, dir, "package.yml")
	for _, value := range []string{"My app", "2.0.0", "Shared code"} {
		if !strings.Contains(manifest, value) {
			t.Fatalf("missing %s", value)
		}
	}
	if !strings.Contains(out.String(), "Enter a semantic version") {
		t.Fatal("did not explain invalid version")
	}
	dir = initProject(t, false)
	build := readFile(t, dir, "build.gradle.kts")
	settings := readFile(t, dir, "settings.gradle.kts")
	if err := initCommand(dir, nil, strings.NewReader("My app\n"), &out, &out); err == nil {
		t.Fatal("accepted incomplete questionnaire")
	}
	if _, err := os.Stat(filepath.Join(dir, "package.yml")); !os.IsNotExist(err) {
		t.Fatal("cancelled init created manifest")
	}
	if readFile(t, dir, "build.gradle.kts") != build || readFile(t, dir, "settings.gradle.kts") != settings {
		t.Fatal("cancelled init edited Gradle")
	}
}

func TestInitPreservesManifestAndUsesCentralRepositories(t *testing.T) {
	dir := initProject(t, false)
	manifest := "# Custom package\nname: Existing\nversion: '3.2.1' # Keep this\ndescription: Existing description\ndependencies: [example:tool:1.0.0]\ncustom: true\n"
	writeFile(t, dir, "package.yml", manifest)
	writeFile(t, dir, "settings.gradle.kts", `pluginManagement {
    repositories { google(); gradlePluginPortal(); mavenCentral() }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories { google() }
}
rootProject.name = "example"
`)
	if _, err := invoke(dir, "init", "-y"); err != nil {
		t.Fatal(err)
	}
	if actual := readFile(t, dir, "package.yml"); actual != manifest {
		t.Fatalf("changed manifest:\n%s", actual)
	}
	if strings.Contains(readFile(t, dir, "build.gradle.kts"), "repositories {") {
		t.Fatal("added forbidden project repositories")
	}
	settings := readFile(t, dir, "settings.gradle.kts")
	if strings.Count(settings, "mavenCentral()") == 2 && strings.Count(settings, "google()") == 2 {
		return
	}
	t.Fatalf("incorrect settings:\n%s", settings)
}

func TestInitSupportsModuleAndInheritedPluginVersion(t *testing.T) {
	root := initProject(t, false)
	writeFile(t, root, "build.gradle.kts", `plugins { id("com.composables.kpm") version "0.3.0" apply false }`)
	dir := filepath.Join(root, "app")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "build.gradle.kts", `plugins { alias(libs.plugins.android.application) }`)
	if _, err := invoke(dir, "init", "-y"); err != nil {
		t.Fatal(err)
	}
	build := readFile(t, dir, "build.gradle.kts")
	if strings.Contains(build, `id("com.composables.kpm") version`) || !strings.Contains(build, `id("com.composables.kpm")`) {
		t.Fatalf("conflicting inherited plugin version:\n%s", build)
	}
	manifest := readFile(t, dir, "package.yml")
	if !strings.Contains(manifest, "name: app") || !strings.Contains(manifest, "version: 0.1.0") {
		t.Fatalf("incorrect module defaults:\n%s", manifest)
	}
	if _, err := os.Stat(filepath.Join(root, "package.yml")); !os.IsNotExist(err) {
		t.Fatal("modified parent manifest")
	}
}

func TestInitUnsupportedLayoutsAndMalformedManifestsLeaveFilesAlone(t *testing.T) {
	for _, test := range []struct{ build, settings, manifest string }{
		{build: `apply(plugin = "org.jetbrains.kotlin.jvm")`},
		{build: `plugins { id("java")`},
		{build: `plugins { id("java") }; version = provider.get()`},
		{build: "plugins { id(\"java\") }\nversion = provider.get()\n"},
		{settings: "import example.Custom\nrootProject.name = \"example\""},
		{manifest: "name: A\nversion: 1.0.0\nversion: 2.0.0\n"},
		{manifest: "name: A\nversion: 1.0.0\n---\nname: B\n"},
		{manifest: "name: A\nversion: invalid\n"},
		{manifest: "name: A\nversion: 1.0.0\ndescription: ''\n"},
		{manifest: "name: A\nversion: 1.0.0\ndependencies: false\n"},
	} {
		dir := initProject(t, false)
		if test.build != "" {
			writeFile(t, dir, "build.gradle.kts", test.build)
		}
		if test.settings != "" {
			writeFile(t, dir, "settings.gradle.kts", test.settings)
		}
		if test.manifest != "" {
			writeFile(t, dir, "package.yml", test.manifest)
		}
		build := readFile(t, dir, "build.gradle.kts")
		settings := readFile(t, dir, "settings.gradle.kts")
		if _, err := invoke(dir, "init", "-y"); err == nil {
			t.Fatalf("accepted unsupported input: %+v", test)
		}
		if readFile(t, dir, "build.gradle.kts") != build || readFile(t, dir, "settings.gradle.kts") != settings {
			t.Fatal("partially edited Gradle")
		}
		if test.manifest != "" {
			if readFile(t, dir, "package.yml") != test.manifest {
				t.Fatal("changed invalid manifest")
			}
		} else if _, err := os.Stat(filepath.Join(dir, "package.yml")); !os.IsNotExist(err) {
			t.Fatal("created manifest for unsupported layout")
		}
	}
}

func TestInitRepositoryAndPluginEditsRespectStringsCommentsAndFilters(t *testing.T) {
	build := `// plugins { id("com.composables.kpm") }
plugins {
    id("java-library")
    // } should not end the plugins block
}
val sample = "repositories { mavenCentral() }"
repositories {
    mavenCentral { content { includeGroup("example") } }
}
`
	settings := `pluginManagement { repositories { mavenCentral(); gradlePluginPortal() } }`
	updated, _, err := setupGradle(build, settings, "0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(updated, `id("com.composables.kpm")`) != 2 {
		t.Fatal("comment mistaken for applied plugin")
	}
	if strings.Contains(updated, "mavenCentral()\n") {
		t.Fatal("bypassed existing repository filter")
	}
	if !strings.Contains(updated, `val sample = "repositories { mavenCentral() }"`) {
		t.Fatal("modified quoted example")
	}
	updated2, _, err := setupGradle(updated, settings, "0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if updated != updated2 {
		t.Fatal("not repeatable")
	}
}

func TestInitActivatesApplyFalsePlugin(t *testing.T) {
	for _, declaration := range []string{`id("com.composables.kpm") version "0.2.0" apply false`, `id 'com.composables.kpm' version '0.2.0' apply false`, `id("com.composables.kpm") version("0.2.0") apply(false)`} {
		build, _, err := setupGradle("plugins { "+declaration+" }", "rootProject.name = \"example\"", "0.3.0")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(build, "apply false") || strings.Contains(build, "apply(false)") || strings.Count(build, "com.composables.kpm") != 1 || !strings.Contains(build, "0.2.0") {
			t.Fatalf("incorrect activation: %s", build)
		}
	}
}

func TestInitInvalidArgumentsAndMissingProject(t *testing.T) {
	for _, args := range [][]string{{"init", "--unknown"}, {"init", "foo"}, {"init", "-y"}} {
		if _, err := invoke(t.TempDir(), args...); err == nil {
			t.Fatalf("accepted %v without project", args)
		}
	}
}

func TestInitUsesReleasedCLIVersionAndPreservesFileModes(t *testing.T) {
	previous := binaryVersion
	binaryVersion = "1.2.3"
	t.Cleanup(func() { binaryVersion = previous })
	dir := initProject(t, false)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(dir, "build.gradle.kts"), 0640); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := invoke(dir, "init", "-y"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, dir, "build.gradle.kts"), `id("com.composables.kpm") version "1.2.3"`) {
		t.Fatal("plugin version does not match CLI")
	}
	info, err := os.Stat(filepath.Join(dir, "build.gradle.kts"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
		t.Fatal("changed file permissions")
	}
}

func TestInitRollsBackWhenAFileCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.yml")
	changes := []initChange{{path: path, after: []byte("name: New\nversion: 0.1.0\n")}, {path: filepath.Join(dir, "missing", "settings.gradle.kts"), after: []byte("new settings")}}
	if err := writeInitChanges(changes); err == nil {
		t.Fatal("expected write failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("left a partially initialized package")
	}
	writeFile(t, dir, "package.yml", "original content\n")
	changes[0].before = []byte("original content\n")
	if err := writeInitChanges(changes); err == nil {
		t.Fatal("expected write failure")
	}
	if readFile(t, dir, "package.yml") != "original content\n" {
		t.Fatal("did not restore existing manifest")
	}
}

func TestInitSupportsWindowsLineEndings(t *testing.T) {
	build := "plugins { id(\"java-library\") }\r\nversion = \"1.2.3\"\r\n"
	updated, _, err := setupGradle(build, "rootProject.name = \"example\"\r\n", "0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(updated, `version = "1.2.3"`) {
		t.Fatal("left version override in CRLF build script")
	}
}
