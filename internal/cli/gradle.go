package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// A package stays local to the current directory; only wrapper lookup walks up.
func findWrapper(dir string) (string, error) {
	name := "gradlew"
	if runtime.GOOS == "windows" {
		name += ".bat"
	}
	for current := dir; ; current = filepath.Dir(current) {
		path := filepath.Join(current, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return "", fmt.Errorf("Gradle wrapper not found in this directory or its parents; start in an existing Gradle project with a wrapper")
}

// Gradle resolves the directory to a project, including settings projectDir mappings.
// Qualifying the task prevents sibling/child packages from sharing a report or publishing.
const selectGradleProject = `gradle.projectsLoaded {
    def path = gradle.startParameter.projectProperties['kpm.projectDir']
    if (path == null || gradle.parent != null) return
    def directory = new File(path).canonicalFile
    def selected = gradle.rootProject.allprojects.find { it.projectDir.canonicalFile == directory }
    if (selected == null) throw new GradleException("KPM: no Gradle project for " + directory)
    def task = gradle.startParameter.projectProperties['kpm.task']
    gradle.startParameter.setTaskNames([(selected.path == ':' ? ':' : selected.path + ':') + task])
}
`

func gradleCommand(dir, task string, args []string) (*exec.Cmd, func(), error) {
	wrapper, err := findWrapper(dir)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {}
	settings, settingsErr := findSettings(dir)
	if filepath.Dir(wrapper) != dir || (settingsErr == nil && filepath.Dir(settings) != dir) {
		file, err := os.CreateTemp("", "kpm-project-*.gradle")
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() { os.Remove(file.Name()) }
		if _, err := file.WriteString(selectGradleProject); err != nil {
			file.Close()
			cleanup()
			return nil, nil, err
		}
		if err := file.Close(); err != nil {
			cleanup()
			return nil, nil, err
		}
		args = append(args, "--init-script", file.Name(), "-Pkpm.projectDir="+dir, "-Pkpm.task="+task)
	} else {
		task = ":" + task
	}
	args = append([]string{task}, args...)
	cmd := exec.Command(wrapper, args...)
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", append([]string{"/d", "/c", wrapper}, args...)...)
	}
	cmd.Dir, cmd.Stdin = dir, os.Stdin
	return cmd, cleanup, nil
}
