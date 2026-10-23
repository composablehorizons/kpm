package scripts

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tarFixture(t *testing.T, version string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	binary := []byte("#!/bin/sh\nprintf '%s\\n' '" + version + "'\n")
	if err := tw.WriteHeader(&tar.Header{Name: "kpm", Mode: 0755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func releaseFixture(t *testing.T, asset string, data []byte, badChecksum bool) *httptest.Server {
	t.Helper()
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	if badChecksum {
		sum = strings.Repeat("0", 64)
	}
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			http.Redirect(out, request, "/tag/v0.4.0", http.StatusFound)
		case "/tag/v0.4.0":
			out.WriteHeader(http.StatusOK)
		case "/download/v0.4.0/" + asset:
			out.Write(data)
		case "/download/v0.4.0/SHA256SUMS":
			fmt.Fprintf(out, "%s  %s\n", sum, asset)
		default:
			http.NotFound(out, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func shellInstaller(t *testing.T, server *httptest.Server, directory string, platform, arch string, args ...string) (string, error) {
	t.Helper()
	tools := t.TempDir()
	// Mock uname only: downloads, extraction, checksums, and installation are real.
	uname := "#!/bin/sh\ncase \"$1\" in -s) echo '" + platform + "';; -m) echo '" + arch + "';; esac\n"
	if err := os.WriteFile(filepath.Join(tools, "uname"), []byte(uname), 0755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", append([]string{"install.sh"}, args...)...)
	command.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"), "KPM_RELEASE_BASE_URL="+server.URL, "KPM_INSTALL_DIR="+directory, "KPM_VERSION=")
	result, err := command.CombinedOutput()
	return string(result), err
}

func TestShellInstallerDetectsPlatformsAndSupportsPinnedVersions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer")
	}
	for _, test := range []struct{ os, arch, asset string }{
		{"Darwin", "arm64", "darwin_arm64"}, {"Darwin", "x86_64", "darwin_amd64"},
		{"Linux", "aarch64", "linux_arm64"}, {"Linux", "x86_64", "linux_amd64"},
	} {
		t.Run(test.asset, func(t *testing.T) {
			server := releaseFixture(t, "kpm_0.4.0_"+test.asset+".tar.gz", tarFixture(t, "0.4.0"), false)
			directory := filepath.Join(t.TempDir(), "bin with spaces")
			output, err := shellInstaller(t, server, directory, test.os, test.arch)
			if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			binary := filepath.Join(directory, "kpm")
			actual, err := exec.Command(binary, "--version").Output()
			if err != nil || strings.TrimSpace(string(actual)) != "0.4.0" {
				t.Fatalf("installed binary: %v %s", err, actual)
			}
			// Reinstalling a pinned version must replace an existing installation safely.
			output, err = shellInstaller(t, server, directory, test.os, test.arch, "--version", "v0.4.0")
			if err != nil {
				t.Fatalf("pinned reinstall: %v\n%s", err, output)
			}
		})
	}
}

func TestShellInstallerLeavesExistingBinaryOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer")
	}
	for _, test := range []struct {
		name, version string
		badChecksum   bool
		args          []string
	}{
		{name: "checksum", version: "0.4.0", badChecksum: true},
		{name: "wrong binary version", version: "0.3.0"},
		{name: "invalid version", version: "0.4.0", args: []string{"--version", "../../bad"}},
		{name: "missing download", version: "0.4.0", args: []string{"--version", "0.5.0"}},
		{name: "missing option value", version: "0.4.0", args: []string{"--version"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := releaseFixture(t, "kpm_0.4.0_linux_amd64.tar.gz", tarFixture(t, test.version), test.badChecksum)
			directory := t.TempDir()
			path := filepath.Join(directory, "kpm")
			if err := os.WriteFile(path, []byte("original"), 0755); err != nil {
				t.Fatal(err)
			}
			output, err := shellInstaller(t, server, directory, "Linux", "x86_64", test.args...)
			if err == nil {
				t.Fatalf("accepted invalid installation:\n%s", output)
			}
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != "original" {
				t.Fatalf("changed existing installation: %v", err)
			}
			leftovers, err := filepath.Glob(filepath.Join(directory, ".kpm-install.*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatal("left staged files behind")
			}
		})
	}
}

func TestShellInstallerRejectsUnsupportedPlatforms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer")
	}
	server := releaseFixture(t, "unused", nil, false)
	for _, test := range []struct{ os, arch string }{{"Linux", "armv7l"}, {"MINGW64_NT", "x86_64"}, {"FreeBSD", "amd64"}} {
		if output, err := shellInstaller(t, server, t.TempDir(), test.os, test.arch); err == nil {
			t.Fatalf("accepted unsupported platform: %s", output)
		}
	}
}

func TestPowerShellInstaller(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows installer runs in the Windows CI job")
	}
	shell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "fixture.go")
	if err := os.WriteFile(source, []byte("package main\nimport \"fmt\"\nfunc main(){fmt.Println(\"0.4.0\")}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "fixture.exe")
	if output, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entry, err := archive.Create("kpm.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	for _, badChecksum := range []bool{false, true} {
		server := releaseFixture(t, "kpm_0.4.0_windows_amd64.zip", buffer.Bytes(), badChecksum)
		target := filepath.Join(t.TempDir(), "bin with spaces")
		command := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", "install.ps1", "-InstallDir", target, "-NoPath")
		command.Env = append(os.Environ(), "KPM_RELEASE_BASE_URL="+server.URL, "KPM_VERSION=", "PROCESSOR_ARCHITECTURE=AMD64", "PROCESSOR_ARCHITEW6432=")
		output, err := command.CombinedOutput()
		if badChecksum {
			if err == nil {
				t.Fatal("accepted checksum mismatch")
			}
			if _, err := os.Stat(filepath.Join(target, "kpm.exe")); !os.IsNotExist(err) {
				t.Fatal("installed unverified executable")
			}
			continue
		}
		if err != nil {
			t.Fatalf("PowerShell install: %v\n%s", err, output)
		}
		actual, err := exec.Command(filepath.Join(target, "kpm.exe"), "--version").Output()
		if err != nil || strings.TrimSpace(string(actual)) != "0.4.0" {
			t.Fatalf("Windows binary: %v %s", err, actual)
		}
	}
}
