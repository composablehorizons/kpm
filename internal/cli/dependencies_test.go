package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func dependencyProject(t *testing.T, manifest string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX Gradle wrapper fixture")
	}
	dir := t.TempDir()
	writeFile(t, dir, "package.yml", manifest)
	script := `#!/bin/sh
report=''
lookup=''
for arg do
  case "$arg" in
    -Pkpm.report=*) report=${arg#*=} ;;
    -Pkpm.lookup=*) lookup=${arg#*=} ;;
  esac
done
printf '%s\n' "$@" >> calls
if [ -f fail-install ] && [ "$1" = :kpmInstall ]; then exit 7; fi
if [ "$1" = :kpmLookup ]; then
  if [ -n "$lookup" ]; then printf '%s' '{"version":"2.0.0"}' > "$report"
  else printf '%s' '{"target":"common"}' > "$report"; fi
elif [ -f test-report ]; then
  cat test-report > "$report"
else
  printf '%s' '{"dependencies":[{"target":"common","scope":"api","id":"example:tool","declared":"1.0.0","current":"1.0.0","wanted":"1.0.0","latest":"2.0.0"}]}' > "$report"
fi
`
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

const dependencyManifest = "# Package comment\nname: Example\nversion: 1.2.3\npublishing:\n  groupId: example\n  artifactId: example\n"

func TestInstallShorthandExpansionAndScopeChanges(t *testing.T) {
	dir := dependencyProject(t, dependencyManifest)
	for _, args := range [][]string{
		{"i", "example:tool:1.0.0"},
		{"install", "example:tool:1.1.0", "--api"},
		{"i", "example:tool:1.2.0"}, // An omitted scope preserves api.
		{"i", "example:android:1.0.0", "--target", "android"},
	} {
		if output, err := invoke(dir, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	p, err := readPackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	deps, err := manifestDependencies(p.document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 || deps[0].Target != "common" || deps[0].Scope != "api" || deps[0].Declared != "1.2.0" || deps[1].Target != "android" {
		t.Fatalf("unexpected declarations: %+v", deps)
	}
	content := readFile(t, dir, "package.yml")
	if !strings.Contains(content, "# Package comment") || !strings.Contains(content, "groupId: example") {
		t.Fatal("lost existing metadata or comments")
	}
	if _, err := invoke(dir, "i", "--implementation", "example:tool:1.3.0"); err != nil {
		t.Fatal(err)
	}
	p, _ = readPackage(dir)
	deps, err = manifestDependencies(p.document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range deps {
		if d.ID == "example:tool" && d.Scope != "implementation" {
			t.Fatal("did not move scope")
		}
	}
}
func TestInstallWithoutVersionAndNoArgumentInstall(t *testing.T) {
	dir := dependencyProject(t, dependencyManifest)
	if _, err := invoke(dir, "i", "example:tool"); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, dir, "package.yml"); !strings.Contains(content, "example:tool") || strings.Contains(content, "example:tool:") {
		t.Fatal("versionless declaration was not preserved")
	}
	if strings.Contains(readFile(t, dir, "calls"), "-Pkpm.lookup=") {
		t.Fatal("versionless install looked up a version")
	}
	before := readFile(t, dir, "package.yml")
	if _, err := invoke(dir, "i"); err != nil {
		t.Fatal(err)
	}
	if readFile(t, dir, "package.yml") != before {
		t.Fatal("no-argument install modified manifest")
	}
	if !strings.Contains(readFile(t, dir, "calls"), "--write-locks") {
		t.Fatal("install did not request lock update")
	}
}
func TestInstallLatestSavesExactVersion(t *testing.T) {
	dir := dependencyProject(t, dependencyManifest)
	if _, err := invoke(dir, "i", "example:tool@latest"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, dir, "package.yml"), "example:tool:2.0.0") {
		t.Fatal("latest release was not saved as an exact version")
	}
	if !strings.Contains(readFile(t, dir, "calls"), "-Pkpm.lookup=example:tool:latest.release") {
		t.Fatal("latest release was not looked up through Gradle")
	}
	if _, err := invoke(dir, "i", "example:tool"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, dir, "package.yml"), "example:tool:") {
		t.Fatal("reinstall did not remove explicit version")
	}
}

func TestFailedInstallRestoresOriginalManifest(t *testing.T) {
	dir := dependencyProject(t, dependencyManifest)
	writeFile(t, dir, "fail-install", "")
	if _, err := invoke(dir, "i", "example:tool:1.0.0"); err == nil {
		t.Fatal("expected resolution failure")
	}
	if readFile(t, dir, "package.yml") != dependencyManifest {
		t.Fatal("failed install changed manifest")
	}
}
func TestUninstallTargetsAndAtomicValidation(t *testing.T) {
	manifest := dependencyManifest + "dependencies:\n  common: [example:tool:1.0.0]\n  android: [example:tool:1.0.0]\n"
	dir := dependencyProject(t, manifest)
	if _, err := invoke(dir, "rm", "example:tool", "missing:thing"); err == nil {
		t.Fatal("expected missing dependency error")
	}
	if readFile(t, dir, "package.yml") != manifest {
		t.Fatal("partial removal saved")
	}
	if _, err := invoke(dir, "uninstall", "--target=android", "example:tool"); err != nil {
		t.Fatal(err)
	}
	p, _ := readPackage(dir)
	deps, err := manifestDependencies(p.document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Target != "common" {
		t.Fatalf("unexpected remaining dependencies: %+v", deps)
	}
	if _, err := invoke(dir, "remove", "example:tool"); err != nil {
		t.Fatal(err)
	}
}
func TestFlatTargetUninstall(t *testing.T) {
	dir := dependencyProject(t, dependencyManifest+"dependencies: [example:tool:1.0.0]\n")
	if _, err := invoke(dir, "rm", "example:tool", "--target", "common"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, dir, "package.yml"), "example:tool") {
		t.Fatal("flat dependency not removed")
	}
}
func TestListOutdatedAndUpdate(t *testing.T) {
	manifest := dependencyManifest + "dependencies:\n  common:\n    api: [example:tool:1.0.0]\n"
	dir := dependencyProject(t, manifest)
	for _, command := range []string{"list", "ls", "outdated"} {
		output, err := invoke(dir, command)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output, "example:tool") {
			t.Fatalf("missing dependency: %s", output)
		}
		if command == "outdated" && !strings.Contains(output, "2.0.0") {
			t.Fatal("missing latest version")
		}
	}
	if _, err := invoke(dir, "update", "example:tool"); err != nil {
		t.Fatal(err)
	}
	if readFile(t, dir, "package.yml") != manifest {
		t.Fatal("update rewrote manifest without --save")
	}
	calls := readFile(t, dir, "calls")
	if !strings.Contains(calls, "--refresh-dependencies") || !strings.Contains(calls, "--update-locks\nexample:tool") {
		t.Fatal("update did not refresh requested locks")
	}
}
func TestInvalidDependencyCommandsDoNotModifyManifest(t *testing.T) {
	for _, args := range [][]string{
		{"i", "broken"}, {"i", "example:tool@next"}, {"i", "example:tool:1@latest"},
		{"rm", "example:tool@latest"}, {"update", "example:tool@latest"}, {"i", "example:tool:"}, {"i", "example:tool:1", "--api", "--implementation"},
		{"i", "--target"}, {"i", "--target=../bad", "example:tool:1"}, {"i", "--api"}, {"list", "example:tool"},
		{"outdated", "--api"}, {"rm"}, {"update", "example:tool:1.0.0"}, {"i", "example:tool:1", "--save"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := dependencyProject(t, dependencyManifest)
			if _, err := invoke(dir, args...); err == nil {
				t.Fatal("expected invalid argument error")
			}
			if readFile(t, dir, "package.yml") != dependencyManifest {
				t.Fatal("invalid command modified manifest")
			}
		})
	}
}
func TestRejectsMalformedDependencyDeclarations(t *testing.T) {
	for _, yaml := range []string{
		"dependencies: false\n", "dependencies: [example:tool:]\n", "dependencies: [example:tool@latest]\n",
		"dependencies:\n  common:\n    testOnly: []\n", "dependencies: [example:tool:1, example:tool:2]\n",
		"dependencies: &deps [example:tool:1]\n", "dependencies:\n  common: [example:tool:1]\n  android: *missing\n",
	} {
		dir := dependencyProject(t, dependencyManifest+yaml)
		if _, err := invoke(dir, "list"); err == nil {
			t.Fatalf("accepted malformed declaration: %s", yaml)
		}
	}
}

func TestUpdateSavePreservesSelectorsAndSavesExactResolvedVersions(t *testing.T) {
	for _, version := range []string{"", "1.0.0", "1.+", "[1.0,2.0)"} {
		t.Run(version, func(t *testing.T) {
			notation := "example:tool"
			if version != "" {
				notation += ":" + version
			}
			dir := dependencyProject(t, dependencyManifest+"dependencies:\n  common:\n    api:\n      - "+notation+"\n")
			writeFile(t, dir, "test-report", `{"dependencies":[{"target":"common","scope":"api","id":"example:tool","declared":"`+version+`","current":"1.1.0","wanted":"1.1.0"}]}`)
			if _, err := invoke(dir, "up", "--save"); err != nil {
				t.Fatal(err)
			}
			expected := version
			if version == "1.0.0" {
				expected = "1.1.0"
			}
			expectedNotation := "example:tool"
			if expected != "" {
				expectedNotation += ":" + expected
			}
			p, err := readPackage(dir)
			if err != nil {
				t.Fatal(err)
			}
			deps, err := manifestDependencies(p.document.Content[0])
			if err != nil || len(deps) != 1 || deps[0].node.Value != expectedNotation {
				t.Fatal("unexpected saved version")
			}
		})
	}
}

func TestFailedUninstallRestoresOriginalManifest(t *testing.T) {
	manifest := dependencyManifest + "dependencies: [example:tool:1.0.0]\n"
	dir := dependencyProject(t, manifest)
	writeFile(t, dir, "fail-install", "")
	if _, err := invoke(dir, "rm", "example:tool"); err == nil {
		t.Fatal("expected resolution error")
	}
	if readFile(t, dir, "package.yml") != manifest {
		t.Fatal("failed uninstall changed manifest")
	}
}
