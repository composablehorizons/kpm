package com.composables.kpm;

import java.util.Map;
import org.gradle.api.GradleException;
import org.gradle.api.file.Directory;
import org.gradle.api.provider.ProviderFactory;
import org.yaml.snakeyaml.LoaderOptions;
import org.yaml.snakeyaml.Yaml;
import org.yaml.snakeyaml.constructor.SafeConstructor;

/** Reads declarations from the current package only. */
public final class PackageManifest {
    /** Reads a package, returning null when no manifest exists. */
    public static Map<?, ?> read(Directory directory, ProviderFactory providers) {
        var file = directory.file("package.yml");
        try {
            String contents = providers.fileContents(file).getAsText().getOrNull();
            if (contents == null) return null;
            var options = new LoaderOptions();
            options.setAllowDuplicateKeys(false);
            Object value = new Yaml(new SafeConstructor(options)).load(contents);
            if (!(value instanceof Map<?, ?> map)) throw new GradleException("Expected a YAML mapping.");
            if (map.containsKey("versions")) throw new GradleException("The versions block is not supported. Write versions directly in plugin and dependency declarations.");
            return map;
        } catch (RuntimeException error) {
            throw new GradleException("Invalid package.yml (" + file + "): " + error.getMessage(), error);
        }
    }
}
