package cli

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Keep development builds usable; release builds inject the matching CLI version.
const developmentPluginVersion = "0.5.1"

func initCommand(dir string, args []string, input io.Reader, out, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var yes bool
	fs.BoolVar(&yes, "yes", false, "Use inferred defaults without questions")
	fs.BoolVar(&yes, "y", false, "Use inferred defaults without questions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("init takes no positional arguments; run it inside an existing Kotlin project")
	}
	build, err := gradleFile(dir, "build")
	if err != nil {
		return err
	}
	_, err = findWrapper(dir)
	if err != nil {
		return err
	}
	settings, err := findSettings(dir)
	if err != nil {
		return err
	}
	buildData, err := os.ReadFile(build)
	if err != nil {
		return err
	}
	settingsData, err := os.ReadFile(settings)
	if err != nil {
		return err
	}
	version := binaryVersion
	if version == "dev" {
		version = developmentPluginVersion
	}
	if _, err := parseVersion(version); err != nil {
		return fmt.Errorf("cannot select a Gradle plugin version from CLI version %q", version)
	}

	// A root apply-false declaration or pluginManagement default already supplies
	// the plugin version. Asking for a second version can conflict on the classpath.
	pluginVersion := version
	if parentBuild, e := gradleFile(filepath.Dir(settings), "build"); e == nil && parentBuild != build {
		if data, e := os.ReadFile(parentBuild); e == nil {
			if hasDirectKpmPlugin(string(data)) {
				pluginVersion = ""
			}
		}
	}
	if block, index, e := findGradleBlock(string(settingsData), "pluginManagement", -1); e == nil && index != -1 {
		if hasDirectKpmPlugin(string(settingsData)[block.open+1 : block.close]) {
			pluginVersion = ""
		}
	}
	// Plan every Gradle edit before writing anything, so unsupported scripts stay intact.
	updatedBuild, updatedSettings, err := setupGradle(string(buildData), string(settingsData), pluginVersion)
	if err != nil {
		return fmt.Errorf("cannot safely configure this Gradle layout: %w\nNo files changed. Manual setup:\n  Create package.yml with a version (for example, 0.1.0).\n  Apply id(\"com.composables.kpm\") version %q in this project's plugins block.\n  Add mavenCentral() to pluginManagement.repositories in %s and to your dependency repositories.\nThen rerun kpm init", err, version, settings)
	}
	document, original, err := initManifest(dir)
	if err != nil {
		return err
	}
	root := document.Content[0]
	if _, err := manifestDependencies(root); err != nil {
		return err
	}
	publishing := field(root, "publishing") != nil
	for _, key := range []string{"groupId", "artifactId", "url", "licenses", "developers", "scm", "issueManagement"} {
		publishing = publishing || field(root, key) != nil
	}
	if publishing && field(root, "description") == nil {
		return fmt.Errorf("package.yml publishing requires a description; add it before running kpm init")
	}
	initialVersion := inferVersion(dir, filepath.Dir(settings), string(buildData))
	reader := bufio.NewReader(input)
	for _, item := range []struct{ key, label, value string }{{"name", "Package name (optional)", ""}, {"version", "Version", initialVersion}, {"description", "Description (optional)", ""}} {
		existing := field(root, item.key)
		if existing != nil {
			if existing.Kind != yaml.ScalarNode || existing.Tag != "!!str" || existing.Anchor != "" {
				return fmt.Errorf("package.yml %s must be a string without an anchor or alias", item.key)
			}
			if strings.TrimSpace(existing.Value) == "" {
				return fmt.Errorf("package.yml %s must not be empty", item.key)
			}
			if item.key == "version" {
				if _, err := parseVersion(existing.Value); err != nil {
					return err
				}
			}
			continue
		}
		value := item.value
		if !yes {
			for {
				fmt.Fprintf(out, "%s", item.label)
				if item.value != "" {
					fmt.Fprintf(out, " (%s)", item.value)
				}
				fmt.Fprint(out, ": ")
				line, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("initialization cancelled before writing files; use kpm init -y to accept defaults")
				}
				value = strings.TrimSpace(line)
				if value == "" {
					value = item.value
				}
				if item.key == "version" {
					if _, err := parseVersion(value); err != nil {
						fmt.Fprintln(out, "Enter a semantic version, such as 0.1.0.")
						continue
					}
				}
				break
			}
		}
		if item.key == "version" || value != "" {
			put(root, item.key, scalar(value))
		}
	}
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	manifest := buffer.Bytes()
	// Leave existing formatting and comments byte-for-byte intact when no fields changed.
	if original != nil {
		var before yaml.Node
		if err := yaml.Unmarshal(original, &before); err != nil {
			return err
		}
		var a, b any
		if err := before.Decode(&a); err != nil {
			return err
		}
		if err := document.Decode(&b); err != nil {
			return err
		}
		oldCanonical, _ := yaml.Marshal(a)
		newCanonical, _ := yaml.Marshal(b)
		if bytes.Equal(oldCanonical, newCanonical) {
			manifest = original
		}
	}
	changes := []initChange{{path: filepath.Join(dir, "package.yml"), before: original, after: manifest}, {path: settings, before: settingsData, after: []byte(updatedSettings)}, {path: build, before: buildData, after: []byte(updatedBuild)}}
	if err := writeInitChanges(changes); err != nil {
		return err
	}
	for _, change := range changes {
		if !bytes.Equal(change.before, change.after) {
			fmt.Fprintf(out, "Updated %s\n", change.path)
		}
	}
	fmt.Fprintln(out, "KPM is configured. Next: kpm i org.jetbrains.kotlinx:kotlinx-coroutines-core@latest")
	return nil
}

func gradleFile(dir, kind string) (string, error) {
	var paths []string
	for _, suffix := range []string{".gradle.kts", ".gradle"} {
		path := filepath.Join(dir, kind+suffix)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			paths = append(paths, path)
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	if len(paths) != 1 {
		return "", fmt.Errorf("expected one %s.gradle(.kts) in %s; run kpm init inside the Kotlin project or module you want to configure", kind, dir)
	}
	return paths[0], nil
}

func findSettings(dir string) (string, error) {
	for current := dir; ; current = filepath.Dir(current) {
		for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
			if _, err := os.Stat(filepath.Join(current, name)); err == nil {
				return gradleFile(current, "settings")
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return "", fmt.Errorf("no settings.gradle(.kts) found; configure this existing Gradle build manually before running kpm init")
}

func inferVersion(dir, root, build string) string {
	sources := []string{build}
	if dir != root {
		if file, err := gradleFile(root, "build"); err == nil {
			if data, err := os.ReadFile(file); err == nil {
				sources = append(sources, string(data))
			}
		}
	}
	for _, current := range []string{dir, root} {
		if data, err := os.ReadFile(filepath.Join(current, "gradle.properties")); err == nil {
			sources = append(sources, string(data))
		}
	}
	pattern := regexp.MustCompile(`(?m)^\s*version\s*(?:=\s*|\s+)["']?([0-9][0-9A-Za-z.+-]*)["']?\s*;?\s*$`)
	for _, source := range sources {
		if code, err := gradleCode(source, false); err == nil {
			source = code
		}
		if match := pattern.FindStringSubmatch(source); match != nil {
			if _, err := parseVersion(match[1]); err == nil {
				return match[1]
			}
		}
	}
	return "0.1.0"
}

func initManifest(dir string) (yaml.Node, []byte, error) {
	path := filepath.Join(dir, "package.yml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mappingNode()}}, nil, nil
	}
	if err != nil {
		return yaml.Node{}, nil, err
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return document, nil, fmt.Errorf("package.yml: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return document, nil, fmt.Errorf("package.yml must contain exactly one YAML document")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode || document.Content[0].Anchor != "" {
		return document, nil, fmt.Errorf("package.yml must contain a mapping without an anchor")
	}
	var values map[string]any
	if err := document.Decode(&values); err != nil {
		return document, nil, fmt.Errorf("package.yml: %w", err)
	}
	return document, data, nil
}

type initChange struct {
	path          string
	before, after []byte
	mode          os.FileMode
}

func writeInitChanges(changes []initChange) error {
	for i := range changes {
		change := &changes[i]
		change.mode = 0644
		info, err := os.Lstat(change.path)
		if err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("refusing to replace non-regular file %s", change.path)
			}
			change.mode = info.Mode().Perm()
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for i, change := range changes {
		if bytes.Equal(change.before, change.after) {
			continue
		}
		if err := atomicInitWrite(change.path, change.after, change.mode); err != nil {
			failures := []error{err}
			for j := i - 1; j >= 0; j-- {
				previous := changes[j]
				if bytes.Equal(previous.before, previous.after) {
					continue
				}
				var undo error
				if previous.before == nil {
					undo = os.Remove(previous.path)
				} else {
					undo = atomicInitWrite(previous.path, previous.before, previous.mode)
				}
				if undo != nil {
					failures = append(failures, fmt.Errorf("restore %s: %w", previous.path, undo))
				}
			}
			return errors.Join(failures...)
		}
	}
	return nil
}

func atomicInitWrite(path string, data []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".kpm-init-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
