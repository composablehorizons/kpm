package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

type dependency struct {
	Target   string `json:"target"`
	Scope    string `json:"scope"`
	ID       string `json:"id"`
	Declared string `json:"declared"`
	Current  string `json:"current"`
	Wanted   string `json:"wanted"`
	Latest   string `json:"latest"`
	node     *yaml.Node
	list     *yaml.Node
}
type dependencyReport struct {
	Target       string       `json:"target"`
	Version      string       `json:"version"`
	Dependencies []dependency `json:"dependencies"`
}

func isDependencyCommand(command string) bool {
	switch command {
	case "install", "i", "uninstall", "remove", "rm", "list", "ls", "outdated", "update", "up":
		return true
	}
	return false
}
func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}
func mappingNode() *yaml.Node  { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }
func sequenceNode() *yaml.Node { return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"} }
func field(node *yaml.Node, name string) *yaml.Node {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == name {
			return node.Content[i+1]
		}
	}
	return nil
}
func put(node *yaml.Node, name string, value *yaml.Node) {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == name {
			node.Content[i+1] = value
			return
		}
	}
	node.Content = append(node.Content, scalar(name), value)
}

var coordinatePart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var dependencyTarget = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

func coordinates(value string, requireVersion bool) (string, string, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 || !coordinatePart.MatchString(parts[0]) || !coordinatePart.MatchString(parts[1]) || (requireVersion && len(parts) != 3) {
		expected := "group:artifact"
		if requireVersion {
			expected += ":version"
		}
		return "", "", fmt.Errorf("invalid dependency %q; use %s", value, expected)
	}
	version := ""
	if len(parts) == 3 {
		version = parts[2]
		if version == "" || strings.ContainsAny(version, " @\t\r\n") || strings.Contains(version, "${") {
			return "", "", fmt.Errorf("invalid version in %q", value)
		}
	}
	return parts[0] + ":" + parts[1], version, nil
}
func manifestDependencies(root *yaml.Node) ([]dependency, error) {
	deps := field(root, "dependencies")
	if deps == nil {
		return nil, nil
	}
	var result []dependency
	var walk func(*yaml.Node, string, string) error
	walk = func(node *yaml.Node, target, scope string) error {
		if node.Anchor != "" || node.Kind == yaml.AliasNode {
			return fmt.Errorf("dependency anchors and aliases are not supported")
		}
		if node.Kind == yaml.SequenceNode {
			for _, item := range node.Content {
				if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || item.Anchor != "" {
					return fmt.Errorf("dependencies must be group:artifact[:version] strings without anchors")
				}
				id, version, err := coordinates(item.Value, false)
				if err != nil {
					return err
				}
				for _, existing := range result {
					if existing.Target == target && existing.ID == id {
						return fmt.Errorf("duplicate dependency %s for target %s", id, target)
					}
				}
				result = append(result, dependency{Target: target, Scope: scope, ID: id, Declared: version, node: item, list: node})
			}
			return nil
		}
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("dependencies must be a list or a mapping")
		}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if scope == "" { // Scope mapping inside a target.
				if key != "api" && key != "implementation" {
					return fmt.Errorf("unsupported dependency scope %q; use api or implementation", key)
				}
				if node.Content[i+1].Kind != yaml.SequenceNode {
					return fmt.Errorf("%s dependencies must be a list", key)
				}
				if err := walk(node.Content[i+1], target, key); err != nil {
					return err
				}
			} else {
				if !dependencyTarget.MatchString(key) {
					return fmt.Errorf("invalid dependency target %q", key)
				}
				nextScope := ""
				if node.Content[i+1].Kind == yaml.SequenceNode {
					nextScope = "implementation"
				}
				if err := walk(node.Content[i+1], key, nextScope); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(deps, "", "implementation"); err != nil {
		return nil, err
	}
	return result, nil
}
func dependencyList(root *yaml.Node, target, scope, defaultTarget string) *yaml.Node {
	deps := field(root, "dependencies")
	if deps == nil {
		deps = sequenceNode()
		put(root, "dependencies", deps)
	}
	if deps.Kind == yaml.SequenceNode && target == "" && scope == "implementation" {
		return deps
	}
	if deps.Kind == yaml.SequenceNode {
		expanded := mappingNode()
		put(expanded, defaultTarget, deps)
		put(root, "dependencies", expanded)
		deps = expanded
	}
	if target == "" {
		target = defaultTarget
	}
	group := field(deps, target)
	if group == nil {
		group = sequenceNode()
		put(deps, target, group)
	}
	if group.Kind == yaml.SequenceNode && scope == "implementation" {
		return group
	}
	if group.Kind == yaml.SequenceNode {
		expanded := mappingNode()
		put(expanded, "implementation", group)
		put(deps, target, expanded)
		group = expanded
	}
	list := field(group, scope)
	if list == nil {
		list = sequenceNode()
		put(group, scope, list)
	}
	return list
}
func removeDependency(d dependency) {
	for i, item := range d.list.Content {
		if item == d.node {
			d.list.Content = append(d.list.Content[:i], d.list.Content[i+1:]...)
			return
		}
	}
}
func gradleDependency(dir, task string, properties map[string]string, refresh bool, out, stderr io.Writer) (dependencyReport, error) {
	var report dependencyReport
	file, err := os.CreateTemp("", "kpm-report-*.json")
	if err != nil {
		return report, err
	}
	file.Close()
	defer os.Remove(file.Name())
	args := []string{"--console=plain", "--no-configuration-cache", "-Pkpm.report=" + file.Name()}
	for key, value := range properties {
		args = append(args, "-Pkpm."+key+"="+value)
	}
	if refresh {
		args = append(args, "--refresh-dependencies")
	}
	if task == "kpmInstall" {
		if ids := properties["packages"]; refresh && ids != "" {
			args = append(args, "--update-locks", ids)
		} else {
			args = append(args, "--write-locks")
		}
	}
	cmd, cleanup, err := gradleCommand(dir, task, args)
	if err != nil {
		return report, err
	}
	defer cleanup()
	cmd.Stdout, cmd.Stderr = stderr, stderr
	if err := cmd.Run(); err != nil {
		return report, fmt.Errorf("Gradle dependency resolution failed: %w", err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("invalid KPM Gradle report (update the KPM plugin): %w", err)
	}
	return report, nil
}
func dependencyCommand(dir string, args []string, out, stderr io.Writer) error {
	command := args[0]
	switch command {
	case "i":
		command = "install"
	case "remove", "rm":
		command = "uninstall"
	case "ls":
		command = "list"
	case "up":
		command = "update"
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "Dependency target")
	api := fs.Bool("api", false, "Expose the dependency")
	impl := fs.Bool("implementation", false, "Internal dependency")
	save := fs.Bool("save", false, "Save resolved exact versions")
	var positional, options []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		options = append(options, arg)
		if arg == "--target" {
			i++
			if i >= len(args) {
				return fmt.Errorf("--target requires a value")
			}
			options = append(options, args[i])
		}
	}
	if err := fs.Parse(options); err != nil {
		return err
	}
	if *api && *impl {
		return fmt.Errorf("choose either --api or --implementation")
	}
	if (*api || *impl) && command != "install" {
		return fmt.Errorf("scope flags are only supported by install")
	}
	if *save && command != "update" {
		return fmt.Errorf("--save is only supported by update")
	}
	if *target != "" && !dependencyTarget.MatchString(*target) {
		return fmt.Errorf("invalid target %q", *target)
	}
	if command == "list" && len(positional) > 0 {
		return fmt.Errorf("list takes no positional arguments")
	}
	if command == "uninstall" && len(positional) == 0 {
		return fmt.Errorf("uninstall requires group:artifact")
	}
	if command == "install" && len(positional) == 0 && (*api || *impl || *target != "") {
		return fmt.Errorf("install options require a dependency")
	}
	for _, value := range positional {
		coordinate := value
		if command == "install" && strings.HasSuffix(value, "@latest") {
			coordinate = strings.TrimSuffix(value, "@latest")
			if strings.Count(coordinate, ":") != 1 {
				return fmt.Errorf("@latest expects group:artifact@latest")
			}
		}
		_, v, err := coordinates(coordinate, false)
		if err != nil {
			return err
		}
		if command != "install" && v != "" {
			return fmt.Errorf("%s expects group:artifact without a version", command)
		}
	}
	p, err := readPackage(dir)
	if err != nil {
		return err
	}
	root := p.document.Content[0]
	deps, err := manifestDependencies(root)
	if err != nil {
		return err
	}
	defaultTargetForFilter := ""
	if *target != "" && command != "install" {
		for _, d := range deps {
			if d.Target == "" {
				info, err := gradleDependency(dir, "kpmLookup", nil, false, out, stderr)
				if err != nil {
					return err
				}
				defaultTargetForFilter = info.Target
				break
			}
		}
	}
	matchesTarget := func(d dependency) bool {
		dt := d.Target
		if dt == "" {
			dt = defaultTargetForFilter
		}
		return *target == "" || dt == *target
	}
	original, err := os.ReadFile(p.path)
	if err != nil {
		return err
	}
	properties := map[string]string{}
	if *target != "" {
		properties["target"] = *target
	}
	if command == "install" && len(positional) > 0 {
		info, err := gradleDependency(dir, "kpmLookup", nil, false, out, stderr)
		if err != nil {
			return err
		}
		defaultTarget := info.Target
		for _, value := range positional {
			latest := strings.HasSuffix(value, "@latest")
			id, version, _ := coordinates(strings.TrimSuffix(value, "@latest"), false)
			if latest {
				lookup := map[string]string{"lookup": id + ":latest.release", "target": *target}
				report, err := gradleDependency(dir, "kpmLookup", lookup, false, out, stderr)
				if err != nil {
					return err
				}
				version = report.Version
			}
			notation := id
			if version != "" {
				notation += ":" + version
			}
			scope := "implementation"
			if *api {
				scope = "api"
			}
			actualTarget := *target
			if actualTarget == "" {
				actualTarget = defaultTarget
			}
			deps, err = manifestDependencies(root)
			if err != nil {
				return err
			}
			var existing *dependency
			for i := range deps {
				dt := deps[i].Target
				if dt == "" {
					dt = defaultTarget
				}
				if dt == actualTarget && deps[i].ID == id {
					existing = &deps[i]
					break
				}
			}
			if existing != nil && !*api && !*impl {
				existing.node.Value = notation
				continue
			}
			if existing != nil && existing.Scope == scope {
				existing.node.Value = notation
				continue
			}
			if existing != nil {
				removeDependency(*existing)
			}
			list := dependencyList(root, *target, scope, defaultTarget)
			list.Content = append(list.Content, scalar(notation))
		}
		if err := p.write(); err != nil {
			return err
		}
		_, err = gradleDependency(dir, "kpmInstall", nil, false, out, stderr)
		if err != nil {
			if restore := os.WriteFile(p.path, original, p.mode); restore != nil {
				return fmt.Errorf("%w; restoring package.yml failed: %v", err, restore)
			}
			return err
		}
		fmt.Fprintln(out, "Dependencies installed and saved to package.yml")
		return nil
	}
	if command == "uninstall" {
		for _, id := range positional {
			found := false
			for _, d := range deps {
				if d.ID == id && matchesTarget(d) {
					removeDependency(d)
					found = true
				}
			}
			if !found {
				return fmt.Errorf("dependency %s was not found for the selected target", id)
			}
		}
		if err := p.write(); err != nil {
			return err
		}
		if _, err := gradleDependency(dir, "kpmInstall", nil, false, out, stderr); err != nil {
			if restore := os.WriteFile(p.path, original, p.mode); restore != nil {
				return fmt.Errorf("%w; restoring package.yml failed: %v", err, restore)
			}
			return err
		}
		fmt.Fprintln(out, "Dependencies removed from package.yml")
		return nil
	}
	if len(positional) > 0 {
		for _, id := range positional {
			found := false
			for _, d := range deps {
				if d.ID == id && matchesTarget(d) {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("dependency %s was not found for the selected target", id)
			}
		}
		properties["packages"] = strings.Join(positional, ",")
	}
	task := "kpmInstall"
	if command == "list" {
		task = "kpmList"
	}
	if command == "outdated" {
		task = "kpmOutdated"
	}
	report, err := gradleDependency(dir, task, properties, command == "update", out, stderr)
	if err != nil {
		return err
	}
	if command == "install" {
		fmt.Fprintln(out, "Dependencies installed")
		return nil
	}
	if command == "update" {
		if *save {
			for _, row := range report.Dependencies {
				for _, d := range deps {
					if d.Target == row.Target && d.Scope == row.Scope && d.ID == row.ID && d.Declared != "" && !strings.ContainsAny(d.Declared, "+[](),") && !strings.HasPrefix(d.Declared, "latest.") {
						d.node.Value = d.ID + ":" + row.Current
					}
				}
			}
			if err := p.write(); err != nil {
				return err
			}
		}
		fmt.Fprintln(out, "Dependencies updated within declared version constraints")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if command == "outdated" {
		fmt.Fprintln(w, "PACKAGE\tTARGET\tSCOPE\tCURRENT\tWANTED\tLATEST")
	} else {
		fmt.Fprintln(w, "PACKAGE\tTARGET\tSCOPE\tDECLARED\tRESOLVED")
	}
	for _, d := range report.Dependencies {
		target := d.Target
		if target == "" {
			target = "default"
		}
		if command == "outdated" {
			if d.Current != d.Latest {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", d.ID, target, d.Scope, d.Current, d.Wanted, d.Latest)
			}
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", d.ID, target, d.Scope, d.Declared, d.Current)
		}
	}
	return w.Flush()
}
