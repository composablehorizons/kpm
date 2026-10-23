//go:build ignore

// Build and package the native CLI without a JavaScript toolchain.
// Run from the repository root: go run ./scripts/build.go
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
}

func run() error {
	output := flag.String("output", "dist", "Release asset directory")
	binaries := flag.String("binaries-dir", "", "Package existing OS-ARCH binaries instead of rebuilding (for historical releases)")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(data))
	if _, err := semver.StrictNewVersion(version); err != nil {
		return fmt.Errorf("VERSION: %w", err)
	}
	if err := os.MkdirAll(*output, 0755); err != nil {
		return err
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "kpm-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	var files []string
	for _, platform := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			executable := "kpm"
			if platform == "windows" {
				executable += ".exe"
			}
			path := filepath.Join(temporary, platform+"-"+arch, executable)
			if *binaries != "" {
				path = filepath.Join(*binaries, platform+"-"+arch, executable)
			} else {
				command := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -X github.com/composablehorizons/kpm/internal/cli.binaryVersion="+version, "-o", path, ".")
				command.Env = append(os.Environ(), "GOOS="+platform, "GOARCH="+arch, "CGO_ENABLED=0")
				command.Stdout, command.Stderr = os.Stdout, os.Stderr
				if err := command.Run(); err != nil {
					return err
				}
			}
			binary, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var archive []byte
			extension := ".tar.gz"
			if platform == "windows" {
				extension = ".zip"
				archive, err = zipArchive(executable, binary, license)
			} else {
				archive, err = tarArchive(executable, binary, license)
			}
			if err != nil {
				return err
			}
			name := "kpm_" + version + "_" + platform + "_" + arch + extension
			if err := os.WriteFile(filepath.Join(*output, name), archive, 0644); err != nil {
				return err
			}
			files = append(files, name)
			fmt.Println("Built", name)
		}
	}
	for _, name := range []string{"install.sh", "install.ps1"} {
		data, err := os.ReadFile(filepath.Join("scripts", name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*output, name), data, 0644); err != nil {
			return err
		}
		files = append(files, name)
	}
	sort.Strings(files)
	var sums strings.Builder
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(*output, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(data), name)
	}
	return os.WriteFile(filepath.Join(*output, "SHA256SUMS"), []byte(sums.String()), 0644)
}

func tarArchive(executable string, binary, license []byte) ([]byte, error) {
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressed)
	for _, entry := range []struct {
		name string
		data []byte
		mode int64
	}{{executable, binary, 0755}, {"LICENSE", license, 0644}} {
		if err := archive.WriteHeader(&tar.Header{Name: entry.name, Mode: entry.mode, Size: int64(len(entry.data)), ModTime: time.Unix(0, 0)}); err != nil {
			return nil, err
		}
		if _, err := archive.Write(entry.data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func zipArchive(executable string, binary, license []byte) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, entry := range []struct {
		name string
		data []byte
		mode os.FileMode
	}{{executable, binary, 0755}, {"LICENSE", license, 0644}} {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetMode(entry.mode)
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(entry.data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
