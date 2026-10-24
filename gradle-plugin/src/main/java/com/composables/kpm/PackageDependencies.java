package com.composables.kpm;

import groovy.json.JsonOutput;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.gradle.api.GradleException;
import org.gradle.api.Project;
import org.gradle.api.artifacts.Configuration;
import org.gradle.api.artifacts.component.ModuleComponentIdentifier;

/** Declares manifest dependencies and delegates all resolution to Gradle. */
final class PackageDependencies {
    record Entry(String target, String scope, String notation) {}
    private final Project project;
    private final List<Entry> entries = new ArrayList<>();

    PackageDependencies(Project project, Object value) {
        this.project = project;
        if (value instanceof List<?>) read("", value);
        else if (value instanceof Map<?, ?> targets) {
            targets.forEach((target, dependencies) -> {
                if (!(target instanceof String name) || !name.matches("[A-Za-z][A-Za-z0-9]*"))
                    throw new GradleException("Dependency targets must be source-set names, such as common, android, or jvm.");
                read(name, dependencies);
            });
        } else if (value != null) throw new GradleException("dependencies must be a list or a mapping of targets.");
        project.afterEvaluate(ignored -> {
            for (Entry entry : entries) project.getDependencies().add(configuration(entry.target(), entry.scope()), entry.notation());
        });
        task("kpmInstall", false);
        task("kpmList", false);
        task("kpmOutdated", true);
        project.getTasks().register("kpmLookup", task -> {
            task.notCompatibleWithConfigurationCache("KPM CLI resolves dependencies using the configured project repositories.");
            task.doLast(ignored -> {
                String target = project.findProperty("kpm.target") == null ? "" : project.property("kpm.target").toString();
                String defaultTarget = defaultTarget();
                if (project.findProperty("kpm.lookup") == null) {
                    report(Map.of("target", defaultTarget));
                    return;
                }
                String notation = project.property("kpm.lookup").toString();
                var base = resolvable(configuration(target, "implementation"));
                report(Map.of("version", resolveVersion(base, notation)));
            });
        });
    }

    private void read(String target, Object value) {
        if (value instanceof List<?> list) readList(target, "implementation", list);
        else if (value instanceof Map<?, ?> scopes) {
            scopes.forEach((scope, dependencies) -> {
                if (!List.of("api", "implementation").contains(scope))
                    throw new GradleException("Unsupported dependency scope '" + scope + "'; use api or implementation.");
                if (!(dependencies instanceof List<?> list)) throw new GradleException("Dependency scope " + scope + " must be a list.");
                readList(target, scope.toString(), list);
            });
        } else throw new GradleException("Dependencies for " + target + " must be a list or a mapping of scopes.");
    }

    private void readList(String target, String scope, List<?> list) {
        for (Object value : list) {
            if (!(value instanceof String notation) || !notation.matches("[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+(:[^\\s:@]+)?") || notation.contains("${"))
                throw new GradleException("Dependencies must use group:artifact[:version] coordinates.");
            String id = dependencyId(notation);
            if (entries.stream().anyMatch(e -> e.target().equals(target) && dependencyId(e.notation()).equals(id)))
                throw new GradleException("Duplicate dependency " + id + " for target " + target);
            entries.add(new Entry(target, scope, notation));
        }
    }

    private String dependencyId(String notation) {
        String[] parts = notation.split(":", 3);
        return parts[0] + ":" + parts[1];
    }

    private String declaredVersion(String notation) {
        String[] parts = notation.split(":", 3);
        return parts.length == 3 ? parts[2] : "";
    }

    private String defaultTarget() {
        return project.getPlugins().hasPlugin("org.jetbrains.kotlin.multiplatform") ? "common"
            : project.getPlugins().hasPlugin("com.android.library") || project.getPlugins().hasPlugin("com.android.application") ? "android" : "jvm";
    }

    private String configuration(String target, String scope) {
        boolean multiplatform = project.getPlugins().hasPlugin("org.jetbrains.kotlin.multiplatform");
        String name;
        if (multiplatform) name = (target.isEmpty() ? "common" : target) + "Main" + Character.toUpperCase(scope.charAt(0)) + scope.substring(1);
        else {
            boolean matches = target.isEmpty()
                || (target.equals("jvm") && (project.getPlugins().hasPlugin("org.jetbrains.kotlin.jvm") || project.getPlugins().hasPlugin("java")))
                || (target.equals("android") && (project.getPlugins().hasPlugin("com.android.library") || project.getPlugins().hasPlugin("com.android.application")));
            if (!matches) throw new GradleException("Dependency target '" + target + "' does not exist in this project.");
            name = scope;
        }
        if (project.getConfigurations().findByName(name) == null)
            throw new GradleException("Dependency configuration '" + name + "' does not exist; apply the Kotlin/Android library plugin and configure the target.");
        return name;
    }

    private List<Configuration> candidates(String declared) {
        var declaration = project.getConfigurations().getByName(declared);
        return project.getConfigurations().stream()
            .filter(c -> c.isCanBeResolved() && c.getHierarchy().contains(declaration))
            .sorted(java.util.Comparator.comparing(Configuration::getName)).toList();
    }

    private Configuration resolvable(String declared) {
        var candidates = candidates(declared);
        if (candidates.isEmpty()) throw new GradleException("No resolvable configuration consumes " + declared + ".");
        return candidates.stream().filter(c -> !c.getName().toLowerCase().contains("test")).findFirst().orElse(candidates.get(0));
    }

    private String selected(Configuration configuration, String id) {
        // Resolve artifacts as well as metadata, just as an install should.
        configuration.resolve();
        return configuration.getIncoming().getResolutionResult().getAllComponents().stream()
            .map(c -> c.getId()).filter(ModuleComponentIdentifier.class::isInstance)
            .map(ModuleComponentIdentifier.class::cast)
            .filter(c -> (c.getGroup() + ":" + c.getModule()).equals(id))
            .map(ModuleComponentIdentifier::getVersion).findFirst()
            .orElseThrow(() -> new GradleException("Could not find resolved dependency " + id + " in " + configuration.getName()));
    }

    private String resolveVersion(Configuration base, String notation) {
        var detached = project.getConfigurations().detachedConfiguration(project.getDependencies().create(notation));
        detached.setTransitive(false);
        detached.getResolutionStrategy().cacheDynamicVersionsFor(0, "seconds");
        base.getAttributes().keySet().forEach(key -> copyAttribute(base, detached, key));
        return selected(detached, notation.substring(0, notation.lastIndexOf(':')));
    }

    private <T> void copyAttribute(Configuration from, Configuration to, org.gradle.api.attributes.Attribute<T> key) {
        to.getAttributes().attribute(key, from.getAttributes().getAttribute(key));
    }

    private void task(String name, boolean outdated) {
        project.getTasks().register(name, task -> {
            task.notCompatibleWithConfigurationCache("KPM CLI resolves dependencies using the configured project repositories.");
            task.doLast(ignored -> {
                String targetFilter = String.valueOf(project.findProperty("kpm.target") == null ? "" : project.property("kpm.target"));
                String idFilter = String.valueOf(project.findProperty("kpm.packages") == null ? "" : project.property("kpm.packages"));
                List<Map<String, String>> rows = new ArrayList<>();
                for (Entry entry : entries) {
                    String id = dependencyId(entry.notation());
                    if (!targetFilter.isEmpty() && !targetFilter.equals(entry.target().isEmpty() ? defaultTarget() : entry.target())) continue;
                    if (!idFilter.isEmpty() && !List.of(idFilter.split(",")).contains(id)) continue;
                    String config = configuration(entry.target(), entry.scope());
                    String current = selected(resolvable(config), id);
                    if (name.equals("kpmInstall")) for (Configuration candidate : candidates(config)) candidate.resolve();
                    var row = new LinkedHashMap<String, String>();
                    row.put("target", entry.target()); row.put("scope", entry.scope()); row.put("id", id);
                    row.put("declared", declaredVersion(entry.notation()));
                    row.put("current", current); row.put("wanted", current);
                    if (outdated) {
                        row.put("wanted", declaredVersion(entry.notation()).isEmpty() ? current : resolveVersion(resolvable(config), entry.notation()));
                        row.put("latest", resolveVersion(resolvable(config), id + ":latest.release"));
                    }
                    rows.add(row);
                }
                if (name.equals("kpmInstall")) {
                    project.getConfigurations().stream().filter(c -> c.isCanBeResolved()
                        && c.getName().matches("(?i).*(compileClasspath|runtimeClasspath|resolvableDependenciesMetadata|compileKlibraries)"))
                        .forEach(Configuration::resolve);
                }
                report(Map.of("dependencies", rows));
            });
        });
    }

    private void report(Object value) {
        try {
            Files.writeString(Path.of(project.property("kpm.report").toString()), JsonOutput.toJson(value));
        } catch (java.io.IOException error) { throw new GradleException("Could not write KPM dependency report", error); }
    }
}
