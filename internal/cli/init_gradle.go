package cli

import (
	"fmt"
	"regexp"
	"strings"
)

type gradleBlock struct {
	name                string
	open, close, parent int
}

// Mask comments and strings while preserving offsets. Braces inside either are
// not Gradle blocks; regular expressions alone cannot safely locate a block end.
func gradleCode(source string, hideStrings bool) (string, error) {
	code := []byte(source)
	blank := func(start, end int) {
		for j := start; j < end; j++ {
			if code[j] != '\n' && code[j] != '\r' {
				code[j] = ' '
			}
		}
	}
	for i := 0; i < len(source); {
		start := i
		if strings.HasPrefix(source[i:], "//") {
			for i < len(source) && source[i] != '\n' {
				i++
			}
			blank(start, i)
		} else if strings.HasPrefix(source[i:], "/*") {
			i += 2
			depth := 1
			for i < len(source) && depth > 0 {
				if strings.HasPrefix(source[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(source[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth != 0 {
				return "", fmt.Errorf("unterminated Gradle comment")
			}
			blank(start, i)
		} else if source[i] == '\'' || source[i] == '"' || source[i] == '`' {
			quote := source[i : i+1]
			if strings.HasPrefix(source[i:], strings.Repeat(quote, 3)) {
				quote = strings.Repeat(quote, 3)
			}
			i += len(quote)
			closed := false
			for i < len(source) {
				if source[i] == '\\' {
					i += 2
					continue
				}
				if strings.HasPrefix(source[i:], quote) {
					i += len(quote)
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "", fmt.Errorf("unterminated Gradle string")
			}
			if hideStrings {
				blank(start, i)
			}
		} else {
			i++
		}
	}
	return string(code), nil
}

func gradleBlocks(source string) ([]gradleBlock, error) {
	code, err := gradleCode(source, true)
	if err != nil {
		return nil, err
	}
	var blocks []gradleBlock
	var stack []int
	identifier := regexp.MustCompile(`([A-Za-z_][A-Za-z_0-9]*)\s*$`)
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '{':
			parent := -1
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			name := ""
			if match := identifier.FindStringSubmatch(code[:i]); match != nil {
				name = match[1]
			}
			blocks = append(blocks, gradleBlock{name: name, open: i, close: -1, parent: parent})
			stack = append(stack, len(blocks)-1)
		case '}':
			if len(stack) == 0 {
				return nil, fmt.Errorf("unbalanced Gradle braces")
			}
			blocks[stack[len(stack)-1]].close = i
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unbalanced Gradle braces")
	}
	return blocks, nil
}

func findGradleBlock(source, name string, parent int) (gradleBlock, int, error) {
	blocks, err := gradleBlocks(source)
	if err != nil {
		return gradleBlock{}, -1, err
	}
	found := -1
	for i, b := range blocks {
		if b.name == name && b.parent == parent {
			if found != -1 {
				return gradleBlock{}, -1, fmt.Errorf("multiple %s blocks require manual setup", name)
			}
			found = i
		}
	}
	if found == -1 {
		return gradleBlock{}, -1, nil
	}
	return blocks[found], found, nil
}

func insertGradle(source string, block gradleBlock, text string) string {
	// Insert at the opening brace to handle single-line blocks and preserve comments.
	return source[:block.open+1] + "\n" + text + "\n" + source[block.open+1:]
}

func ensureRepositories(source, parent string, methods []string) (string, error) {
	parentIndex := -1
	var parentBlock gradleBlock
	var err error
	if parent != "" {
		parentBlock, parentIndex, err = findGradleBlock(source, parent, -1)
		if err != nil {
			return "", err
		}
		if parentIndex == -1 {
			return "", fmt.Errorf("missing %s block", parent)
		}
	}
	block, index, err := findGradleBlock(source, "repositories", parentIndex)
	if err != nil {
		return "", err
	}
	indent := "    "
	if parent != "" {
		indent += "    "
	}
	var additions []string
	for _, method := range methods {
		present := false
		if index != -1 {
			code, err := gradleCode(source, true)
			if err != nil {
				return "", err
			}
			// Preserve existing repository filters and credentials.
			blocks, err := gradleBlocks(source)
			if err != nil {
				return "", err
			}
			pattern := regexp.MustCompile(`\b` + method + `\s*(?:\(\s*\)|\{)`)
			for _, match := range pattern.FindAllStringIndex(code[block.open+1:block.close], -1) {
				offset := block.open + 1 + match[0]
				nested := false
				for _, child := range blocks {
					if child.open > block.open && child.open < offset && child.close > offset {
						nested = true
					}
				}
				if !nested {
					present = true
				}
			}
		}
		if !present {
			additions = append(additions, indent+method+"()")
		}
	}
	if len(additions) == 0 {
		return source, nil
	}
	if index != -1 {
		return insertGradle(source, block, strings.Join(additions, "\n")), nil
	}
	text := "repositories {\n" + strings.Join(additions, "\n") + "\n}"
	if parent != "" {
		text = "    repositories {\n" + strings.Join(additions, "\n") + "\n    }"
		return insertGradle(source, parentBlock, text), nil
	}
	return source + "\n" + text + "\n", nil
}

var kpmPlugin = regexp.MustCompile(`\bid\s*(?:\(\s*["']com\.composables\.kpm["']\s*\)|\s+["']com\.composables\.kpm["'])`)

func kpmDeclaration(source string) []int {
	code, err := gradleCode(source, false)
	if err != nil {
		return nil
	}
	masked, err := gradleCode(source, true)
	if err != nil {
		return nil
	}
	for _, match := range kpmPlugin.FindAllStringIndex(code, -1) {
		if strings.HasPrefix(masked[match[0]:], "id") {
			return match
		}
	}
	return nil
}

func hasDirectKpmPlugin(source string) bool {
	block, index, err := findGradleBlock(source, "plugins", -1)
	return err == nil && index != -1 && kpmDeclaration(source[block.open+1:block.close]) != nil
}

func setupGradle(build, settings, version string) (string, string, error) {
	// Only direct project blocks are edited. Convention plugins and computed script
	// layouts should be configured explicitly rather than rewritten speculatively.
	plugins, index, err := findGradleBlock(build, "plugins", -1)
	if err != nil {
		return "", "", err
	}
	if index == -1 {
		return "", "", fmt.Errorf("no direct plugins block in the build file")
	}
	commentsRemoved, err := gradleCode(build, false)
	if err != nil {
		return "", "", err
	}
	declaration := "id(\"com.composables.kpm\")"
	if version != "" {
		declaration += " version \"" + version + "\""
	}
	pluginSource := commentsRemoved[plugins.open+1 : plugins.close]
	if match := kpmDeclaration(pluginSource); match != nil {
		// An apply-false declaration makes the plugin available but does not enable it.
		rest := pluginSource[match[1]:]
		applyFalse := regexp.MustCompile(`^\s*(?:version\s*(?:\(\s*["'][^"']+["']\s*\)|["'][^"']+["']))?\s*(apply\s*(?:false|\(\s*false\s*\)))`)
		firstLine := strings.SplitN(strings.SplitN(rest, "\n", 2)[0], ";", 2)[0]
		if applyFalse.FindStringSubmatchIndex(rest) == nil && regexp.MustCompile(`\bapply\s*(?:false|\(\s*false\s*\))`).MatchString(firstLine) {
			return "", "", fmt.Errorf("KPM has an apply-false declaration with a computed version; apply it manually")
		}
		if found := applyFalse.FindStringSubmatchIndex(rest); found != nil {
			start := plugins.open + 1 + match[1] + found[2]
			end := plugins.open + 1 + match[1] + found[3]
			build = build[:start] + build[end:]
		}
	} else {
		build = insertGradle(build, plugins, "    "+declaration)
	}
	_, pmIndex, err := findGradleBlock(settings, "pluginManagement", -1)
	if err != nil {
		return "", "", err
	}
	if pmIndex == -1 {
		// Imports must stay before declarations in Kotlin scripts.
		code, err := gradleCode(settings, true)
		if err != nil {
			return "", "", err
		}
		if regexp.MustCompile(`(?m)^\s*(import|package)\b|@file:`).MatchString(code) {
			return "", "", fmt.Errorf("settings imports or file annotations require manual pluginManagement setup")
		}
		settings = "pluginManagement {\n    repositories {\n        mavenCentral()\n        gradlePluginPortal()\n    }\n}\n\n" + settings
	} else {
		settings, err = ensureRepositories(settings, "pluginManagement", []string{"mavenCentral", "gradlePluginPortal"})
		if err != nil {
			return "", "", err
		}
	}
	_, drmIndex, err := findGradleBlock(settings, "dependencyResolutionManagement", -1)
	if err != nil {
		return "", "", err
	}
	if drmIndex != -1 {
		settings, err = ensureRepositories(settings, "dependencyResolutionManagement", []string{"mavenCentral"})
	} else {
		build, err = ensureRepositories(build, "", []string{"mavenCentral"})
	}
	if err != nil {
		return "", "", err
	}
	build, err = adoptGradleVersion(build)
	if err != nil {
		return "", "", err
	}
	return build, settings, nil
}

// The plugin sets project.version from the manifest. A later script assignment
// would silently override future kpm version changes.
func adoptGradleVersion(source string) (string, error) {
	code, err := gradleCode(source, false)
	if err != nil {
		return "", err
	}
	masked, err := gradleCode(source, true)
	if err != nil {
		return "", err
	}
	blocks, err := gradleBlocks(source)
	if err != nil {
		return "", err
	}
	// Locate assignments in code, never in multiline strings or comments.
	assignments := regexp.MustCompile(`(?m)(?:^|;)[ \t]*(?:project\.)?version[ \t]*(?:=[ \t]*|[ \t]+)`).FindAllStringIndex(masked, -1)
	literal := regexp.MustCompile(`^["']([^"']+)["'][ \t\r]*;?[ \t\r]*$`)
	for i := len(assignments) - 1; i >= 0; i-- {
		match := assignments[i]
		nested := false
		for _, block := range blocks {
			if block.open < match[0] && block.close > match[0] {
				nested = true
			}
		}
		if nested {
			continue
		}
		if source[match[0]] == ';' {
			return "", fmt.Errorf("inline project version assignment requires manual migration to package.yml")
		}
		end := strings.IndexByte(code[match[1]:], '\n')
		if end == -1 {
			end = len(code)
		} else {
			end += match[1]
		}
		// Whitespace in the mask includes the literal; find the value after '=' or
		// the version keyword using the original source with comments removed.
		line := code[match[0]:end]
		prefix := regexp.MustCompile(`^[ \t]*(?:project\.)?version[ \t]*(?:=[ \t]*|[ \t]+)`).FindStringIndex(line)
		value := literal.FindStringSubmatch(line[prefix[1]:])
		if value == nil {
			return "", fmt.Errorf("project version is computed; move its value to package.yml and remove the build file's version assignment")
		}
		if _, err := parseVersion(value[1]); err != nil {
			return "", fmt.Errorf("existing project version %q is not semantic; move it to package.yml as a semantic version", value[1])
		}
		// Keep any original comment following the literal.
		for end > match[0] && (code[end-1] == ' ' || code[end-1] == '\t' || code[end-1] == '\r') {
			end--
		}
		source = source[:match[0]] + "// Version is managed in package.yml." + source[end:]
	}
	return source, nil
}
