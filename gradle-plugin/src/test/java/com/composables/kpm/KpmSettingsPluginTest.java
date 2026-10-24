package com.composables.kpm;

import static org.junit.Assert.*;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import org.gradle.testkit.runner.GradleRunner;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

public class KpmSettingsPluginTest {
    @Rule public TemporaryFolder temporary = new TemporaryFolder();

    private void pluginRepository(Path root, String version) throws Exception {
        Path directory = root.resolve("repo/example/library/example.library.gradle.plugin/" + version);
        Files.createDirectories(directory);
        String name = "example.library.gradle.plugin-" + version;
        Files.writeString(directory.resolve(name + ".pom"), """
            <project><modelVersion>4.0.0</modelVersion><groupId>example.library</groupId>
            <artifactId>example.library.gradle.plugin</artifactId><version>%s</version></project>
            """.formatted(version));
        try (var jar = new JarOutputStream(Files.newOutputStream(directory.resolve(name + ".jar")))) {
            jar.putNextEntry(new JarEntry("META-INF/gradle-plugins/example.library.properties"));
            jar.write(("implementation-class=" + ExamplePlugin.class.getName()).getBytes(java.nio.charset.StandardCharsets.UTF_8));
            jar.closeEntry();
            String resource = ExamplePlugin.class.getName().replace('.', '/') + ".class";
            jar.putNextEntry(new JarEntry(resource));
            try (var input = getClass().getClassLoader().getResourceAsStream(resource)) {
                input.transferTo(jar);
            }
            jar.closeEntry();
        }
    }

    private Path project() throws Exception {
        Path root = temporary.newFolder().toPath();
        pluginRepository(root, "1.0");
        Files.writeString(root.resolve("settings.gradle.kts"), """
            pluginManagement { repositories { maven { url = uri("repo") } } }
            plugins { id("com.composables.kpm.settings") }
            rootProject.name = "example"
            """);
        Files.writeString(root.resolve("package.yml"), """
            name: Example
            version: 1.0.0
            plugins:
              library: example.library:1.0
            """);
        Files.writeString(root.resolve("build.gradle.kts"), """
            plugins {
                id("com.composables.kpm")
            }
            java { withSourcesJar() }
            check(plugins.hasPlugin("example.library"))
            check(version.toString() == "1.0.0")
            """);
        return root;
    }

    private GradleRunner runner(Path root, String... arguments) {
        return GradleRunner.create().withProjectDir(root.toFile()).withPluginClasspath().withArguments(arguments);
    }

    @Test public void resolvesManifestPluginsWithTypedAccessorsAndTracksManifestChanges() throws Exception {
        Path root = project();
        runner(root, "buildEnvironment", "--configuration-cache").build();
        assertTrue(runner(root, "buildEnvironment", "--configuration-cache").build().getOutput().contains("Reusing configuration cache"));
        pluginRepository(root, "1.1");
        Files.writeString(root.resolve("package.yml"), Files.readString(root.resolve("package.yml")).replace("example.library:1.0", "example.library:1.1"));
        String output = runner(root, "buildEnvironment", "--configuration-cache").build().getOutput();
        assertFalse(output.contains("Reusing configuration cache"));
        assertTrue(output.contains("example.library:example.library.gradle.plugin:1.1"));
    }

    @Test public void keepsPluginDeclarationsLocalToCustomProjectDirectories() throws Exception {
        Path root = project();
        Files.writeString(root.resolve("settings.gradle.kts"), Files.readString(root.resolve("settings.gradle.kts")) + """
            include(":child")
            project(":child").projectDir = file("modules/custom")
            """);
        Path child = Files.createDirectories(root.resolve("modules/custom"));
        Files.writeString(child.resolve("package.yml"), """
            name: Child
            version: 1.0.0
            plugins:
              library: java-library
            """);
        Files.writeString(child.resolve("build.gradle.kts"), """
            plugins { id("com.composables.kpm") }
            java { withSourcesJar() }
            check(!plugins.hasPlugin("example.library"))
            """);
        runner(root, ":child:sourcesJar").build();
    }

    @Test public void appliesDeclaredPluginsWithoutRepeatingThemInTheBuild() throws Exception {
        Path root = project();
        Files.writeString(root.resolve("build.gradle.kts"), """
            plugins { id("com.composables.kpm") }
            check(plugins.hasPlugin("example.library"))
            """);
        runner(root, "help").build();
    }

    @Test public void supportsVersionlessPluginsFromGradleAndTheExistingClasspath() throws Exception {
        Path root = project();
        Files.writeString(root.resolve("package.yml"), """
            name: Example
            version: 1.0.0
            plugins:
              java-library: java-library
              metadata: com.composables.kpm
            """);
        Files.writeString(root.resolve("build.gradle.kts"), """
            plugins {
                id("com.composables.kpm")
            }
            java { withSourcesJar() }
            check(version.toString() == "1.0.0")
            """);
        runner(root, "sourcesJar").build();
    }

    @Test public void detectsNewManifestsWhenReusingConfigurationCache() throws Exception {
        Path root = project();
        String manifest = Files.readString(root.resolve("package.yml"));
        Files.delete(root.resolve("package.yml"));
        Files.writeString(root.resolve("build.gradle.kts"), "plugins { `java-library` }\n");
        runner(root, "help", "--configuration-cache").build();
        assertTrue(runner(root, "help", "--configuration-cache").build().getOutput().contains("Reusing configuration cache"));
        Files.writeString(root.resolve("package.yml"), manifest);
        assertFalse(runner(root, "help", "--configuration-cache").build().getOutput().contains("Reusing configuration cache"));
    }

    @Test public void rejectsSharedVersionBlocks() throws Exception {
        Path root = project();
        Files.writeString(root.resolve("package.yml"), """
            version: 1.0.0
            versions:
              library: {from: shared.yml}
            """);
        assertTrue(runner(root, "help").buildAndFail().getOutput().contains("The versions block is not supported"));
    }

    @Test public void rejectsVersionReferencesInCoordinates() throws Exception {
        Path root = project();
        Files.writeString(root.resolve("package.yml"), """
            version: 1.0.0
            plugins:
              java: java-library
            dependencies:
              - example:tool:${versions.tool}
            """);
        assertTrue(runner(root, "help").buildAndFail().getOutput().contains("Dependencies must use group:artifact[:version]"));
        Files.writeString(root.resolve("package.yml"), """
            version: 1.0.0
            plugins:
              library: example.library:${versions.library}
            """);
        assertTrue(runner(root, "help").buildAndFail().getOutput().contains("must be a plugin.id[:version] string"));
    }

    @Test public void managesBuildSrcPluginsAndDependenciesWithInlineVersions() throws Exception {
        Path root = project();
        Files.writeString(root.resolve("package.yml"), """
            name: Example
            version: 1.0.0
            plugins:
              convention: example.convention
            """);
        Files.writeString(root.resolve("build.gradle.kts"), """
            plugins { id("com.composables.kpm") }
            java { withSourcesJar() }
            check(plugins.hasPlugin("example.convention"))
            afterEvaluate { check(configurations.implementation.get().dependencies.isEmpty()) }
            """);
        Path buildSrc = Files.createDirectories(root.resolve("buildSrc"));
        Files.writeString(buildSrc.resolve("settings.gradle.kts"), """
            plugins { id("com.composables.kpm.settings") }
            """);
        Files.writeString(buildSrc.resolve("package.yml"), """
            version: 1.0.0
            plugins:
              gradle: java-gradle-plugin
            dependencies:
              - example.library:example.library.gradle.plugin:1.0
            """);
        Files.writeString(buildSrc.resolve("build.gradle.kts"), """
            plugins { id("com.composables.kpm") }
            repositories { maven { url = uri("../repo") } }
            gradlePlugin {
                plugins {
                    create("convention") {
                        id = "example.convention"
                        implementationClass = "ConventionPlugin"
                    }
                }
            }
            """);
        Path sources = Files.createDirectories(buildSrc.resolve("src/main/java"));
        Files.writeString(sources.resolve("ConventionPlugin.java"), """
            import org.gradle.api.Plugin;
            import org.gradle.api.Project;
            public class ConventionPlugin implements Plugin<Project> {
                public void apply(Project project) {
                    new com.composables.kpm.ExamplePlugin().apply(project);
                }
            }
            """);
        runner(root, "sourcesJar", "--configuration-cache").build();
        assertTrue(runner(root, "sourcesJar", "--configuration-cache").build().getOutput().contains("Reusing configuration cache"));
    }

    @Test public void rejectsInvalidDeclarationsWithoutAnApplyOption() {
        for (Object invalid : new Object[] {"example.library:1.0", Map.of("library", "example.library:"),
            Map.of("library", Map.of("id", "example.library", "version", "1.0", "apply", false)),
            Map.of("bad alias", "example.library:1.0"), Map.of("settings", "com.composables.kpm.settings"),
            Map.of("foo-bar", "example.library:1.0", "foo_bar", "example.library:1.0")}) {
            assertThrows(org.gradle.api.GradleException.class, () -> PackagePlugins.parse(invalid));
        }
        assertEquals(new PackagePlugins.Declaration("example.library", "1.0"),
            PackagePlugins.parse(Map.of("my-library", "example.library:1.0")).get("my-library"));
    }
}
