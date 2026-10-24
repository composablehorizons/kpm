package com.composables.kpm;

import org.gradle.api.Plugin;
import org.gradle.api.initialization.ProjectDescriptor;
import org.gradle.api.initialization.Settings;

/** Resolves manifest plugins before Kotlin DSL compiles project scripts. */
public final class KpmSettingsPlugin implements Plugin<Settings> {
    @Override public void apply(Settings settings) {
        // TestKit and convention builds can already supply the project plugin.
        if (getClass().getClassLoader().getResource("META-INF/gradle-plugins/com.composables.kpm.properties") == null) {
            var properties = new java.util.Properties();
            try (var input = getClass().getResourceAsStream("/kpm-version.properties")) {
                properties.load(input);
            } catch (java.io.IOException error) {
                throw new org.gradle.api.GradleException("Could not read the KPM plugin version.", error);
            }
            settings.getPluginManagement().getPlugins().id("com.composables.kpm").version(properties.getProperty("version"));
        }
        settings.getGradle().settingsEvaluated(ignored -> configure(settings, settings.getRootProject()));
    }

    private static void configure(Settings settings, ProjectDescriptor descriptor) {
        var directory = settings.getLayout().getSettingsDirectory().dir(descriptor.getProjectDir().getAbsolutePath());
        var values = PackageManifest.read(directory, settings.getProviders());
        if (values != null && values.containsKey("plugins")) {
            var plugins = PackagePlugins.parse(values.get("plugins"));
            settings.getGradle().beforeProject(project -> {
                if (!project.getPath().equals(descriptor.getPath())) return;
                project.getBuildscript().getRepositories().addAll(settings.getPluginManagement().getRepositories());
                plugins.values().stream().filter(plugin -> !plugin.version().isEmpty()).forEach(plugin ->
                    project.getBuildscript().getDependencies().add("classpath",
                        plugin.id() + ":" + plugin.id() + ".gradle.plugin:" + plugin.version()));
            });
        }
        descriptor.getChildren().forEach(child -> configure(settings, child));
    }
}
