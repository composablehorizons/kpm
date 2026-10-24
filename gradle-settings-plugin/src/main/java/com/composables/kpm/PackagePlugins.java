package com.composables.kpm;

import java.util.LinkedHashMap;
import java.util.HashSet;
import java.util.Map;
import org.gradle.api.GradleException;

/** Plugin declarations shared by settings resolution and project application. */
public final class PackagePlugins {
    /** A plugin ID and optional version; an empty version uses the existing classpath. */
    public record Declaration(String id, String version) {}

    /** Validates plugin declarations while preserving their application order. */
    public static Map<String, Declaration> parse(Object value) {
        if (!(value instanceof Map<?, ?> entries)) {
            throw new GradleException("plugins must be a mapping of names to plugin.id[:version] strings.");
        }
        var result = new LinkedHashMap<String, Declaration>();
        var ids = new HashSet<String>();
        entries.forEach((key, coordinate) -> {
            if (!(key instanceof String alias) || !alias.matches("[a-z][a-zA-Z0-9]*([._-][a-zA-Z0-9]+)*")) {
                throw new GradleException("Invalid plugin name '" + key + "'. Use letters, numbers, and single '-', '_', or '.' separators.");
            }
            if (!(coordinate instanceof String text) || !text.matches("[A-Za-z0-9_][A-Za-z0-9_.-]*(?::[^\\s:]+)?")) {
                throw new GradleException("Plugin '" + alias + "' must be a plugin.id[:version] string.");
            }
            int separator = text.indexOf(':');
            var declaration = separator < 0 ? new Declaration(text, "")
                : new Declaration(text.substring(0, separator), text.substring(separator + 1));
            if (declaration.id().equals("com.composables.kpm.settings")) {
                throw new GradleException("Declare com.composables.kpm.settings in settings.gradle(.kts); settings plugins cannot be applied to projects.");
            }
            if (!ids.add(declaration.id())) throw new GradleException("Duplicate plugin ID '" + declaration.id() + "'. Declare each plugin once per package.");
            result.put(alias, declaration);
        });
        return result;
    }
}
