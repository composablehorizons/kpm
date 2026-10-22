package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var binaryVersion = "dev"

const usage = `kpm — manage dependencies, version packages, and publish to Maven Central

Usage:
  kpm --version
  kpm version [patch|minor|major|VERSION|from-git]
  kpm publish [--dry-run]
  kpm install|i [group:artifact[:version] ...] [--target TARGET] [--api|--implementation]
  kpm uninstall|remove|rm group:artifact [...] [--target TARGET]
  kpm list|ls [--target TARGET]
  kpm outdated [group:artifact ...] [--target TARGET]
  kpm update|up [group:artifact ...] [--target TARGET] [--save]

Dependency commands use the project's Gradle wrapper and repositories.
install defaults to implementation in commonMain (KMP) or the main source set.
update refreshes resolution within declared Gradle version constraints.
--save also saves resolved exact versions for dependencies already pinned exactly.

Version options (before or after the version):
  --no-git-tag-version      Update package.yml without a commit or tag
  --allow-same-version      Allow setting the existing version
  --tag-version-prefix v   Prefix for Git tags (default: v)
  -m, --message 'Release %s'  Commit/tag message (default: version)

With no version argument, prints the package version.
Version changes require a clean Git tree when creating a commit and tag.
Run package commands from the directory containing package.yml.
Publishing uses the project's Gradle wrapper and existing Gradle credentials.
--dry-run asks Gradle to show tasks without executing them or uploading.
`

func Main() {
	dir, err := os.Getwd()
	if err == nil {
		err = run(dir, os.Args[1:], os.Stdout, os.Stderr)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "kpm:", err)
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() > 0 {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}

func run(dir string, args []string, out, stderr io.Writer) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(out, binaryVersion)
		return nil
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = io.WriteString(out, usage)
		return nil
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		_, _ = io.WriteString(out, usage)
		return nil
	}
	if !isDependencyCommand(args[0]) && args[0] != "version" && args[0] != "publish" {
		return fmt.Errorf("unknown command %q; use kpm --help", args[0])
	}
	root, err := findPackage(dir)
	if err != nil {
		return err
	}
	if isDependencyCommand(args[0]) {
		return dependencyCommand(root, args, out, stderr)
	}
	if args[0] == "version" {
		return versionCommand(root, args[1:], out, stderr)
	}
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "Show Gradle tasks without publishing")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("publish takes no positional arguments")
	}
	metadata, err := readPackage(root)
	if err != nil {
		return err
	}
	wrapper := filepath.Join(root, "gradlew")
	if runtime.GOOS == "windows" {
		wrapper += ".bat"
	}
	if _, err := os.Stat(wrapper); err != nil {
		return fmt.Errorf("Gradle wrapper: %w", err)
	}
	gradleArgs := []string{"publishAllPublicationsToMavenCentral", "--no-configuration-cache"}
	if *dryRun {
		gradleArgs = append(gradleArgs, "--dry-run")
	}
	fmt.Fprintf(out, "Publishing %s through Gradle", metadata.version.Value)
	if *dryRun {
		fmt.Fprint(out, " (dry run; no upload)")
	}
	fmt.Fprintln(out)
	cmd := exec.Command(wrapper, gradleArgs...)
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", append([]string{"/d", "/c", wrapper}, gradleArgs...)...)
	}
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = root, os.Stdin, out, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Gradle publishing failed: %w", err)
	}
	return nil
}

func findPackage(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "package.yml")); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no package.yml in the current directory; run KPM from your package directory")
		}
		return "", err
	}
	return dir, nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(data)), err)
	}
	return strings.TrimSpace(string(data)), nil
}
