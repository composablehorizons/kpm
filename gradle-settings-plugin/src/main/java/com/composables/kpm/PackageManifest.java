package com.composables.kpm;

import java.nio.file.Path;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.regex.Pattern;
import org.gradle.api.GradleException;
import org.gradle.api.file.Directory;
import org.gradle.api.provider.ProviderFactory;
import org.yaml.snakeyaml.LoaderOptions;
import org.yaml.snakeyaml.Yaml;
import org.yaml.snakeyaml.constructor.SafeConstructor;

/** Reads package-local declarations and explicit shared version references. */
public final class PackageManifest {
    private static final Pattern REFERENCE = Pattern.compile("\\$\\{versions\\.([A-Za-z][A-Za-z0-9_-]*)}");
    private final Directory directory;
    private final ProviderFactory providers;
    private final Map<Path, Map<?, ?>> documents = new HashMap<>();
    private final Set<String> resolving = new HashSet<>();

    private PackageManifest(Directory directory, ProviderFactory providers) {
        this.directory = directory;
        this.providers = providers;
    }

    /** Reads a package and expands coordinate references, returning null when no manifest exists. */
    public static Map<?, ?> read(Directory directory, ProviderFactory providers) {
        var reader = new PackageManifest(directory, providers);
        Path file = directory.getAsFile().toPath().resolve("package.yml").toAbsolutePath().normalize();
        var document = reader.document(file, false);
        if (document == null) return null;
        var result = new LinkedHashMap<Object, Object>(document);
        for (String field : List.of("plugins", "dependencies")) {
            if (document.containsKey(field)) result.put(field, reader.expand(document.get(field), file));
        }
        return result;
    }

    private Map<?, ?> document(Path file, boolean required) {
        if (documents.containsKey(file)) return documents.get(file);
        try {
            String contents = providers.fileContents(directory.file(file.toString())).getAsText().getOrNull();
            if (contents == null) {
                if (required) throw new GradleException("Shared version source does not exist: " + file);
                return null;
            }
            var options = new LoaderOptions();
            options.setAllowDuplicateKeys(false);
            Object value = new Yaml(new SafeConstructor(options)).load(contents);
            if (!(value instanceof Map<?, ?> map)) throw new GradleException("Expected a YAML mapping.");
            documents.put(file, map);
            return map;
        } catch (RuntimeException error) {
            throw new GradleException("Invalid package.yml (" + file + "): " + error.getMessage(), error);
        }
    }

    private String version(Path file, String name) {
        String key = file + "#" + name;
        if (!resolving.add(key)) throw new GradleException("Circular version reference: " + key);
        try {
            Object definitions = document(file, true).get("versions");
            if (!(definitions instanceof Map<?, ?> versions) || !versions.containsKey(name)) {
                throw new GradleException("Missing versions." + name + " in " + file);
            }
            Object value = versions.get(name);
            if (value instanceof String text && !text.isBlank()) return interpolate(text, file);
            if (value instanceof Map<?, ?> source && source.size() == 1
                && source.get("from") instanceof String path && !path.isBlank()) {
                return version(file.getParent().resolve(path).normalize(), name);
            }
            throw new GradleException("versions." + name + " in " + file + " must be a non-empty string or {from: relative/package.yml}.");
        } finally {
            resolving.remove(key);
        }
    }

    private String interpolate(String text, Path file) {
        String expanded = REFERENCE.matcher(text).replaceAll(match ->
            java.util.regex.Matcher.quoteReplacement(version(file, match.group(1))));
        if (expanded.contains("${")) throw new GradleException("Invalid version reference in '" + text + "' in " + file + ". Use ${versions.name}.");
        return expanded;
    }

    private Object expand(Object value, Path file) {
        if (value instanceof String text) return interpolate(text, file);
        if (value instanceof List<?> list) {
            var result = new ArrayList<>();
            list.forEach(item -> result.add(expand(item, file)));
            return result;
        }
        if (value instanceof Map<?, ?> map) {
            var result = new LinkedHashMap<>();
            map.forEach((key, item) -> result.put(key, expand(item, file)));
            return result;
        }
        return value;
    }
}
