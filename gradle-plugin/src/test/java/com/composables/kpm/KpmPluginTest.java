package com.composables.kpm;

import static org.junit.Assert.*;

import java.nio.file.Files;
import java.nio.file.Path;
import org.gradle.testkit.runner.GradleRunner;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

public class KpmPluginTest {
    @Rule public TemporaryFolder temporary = new TemporaryFolder();
    private static final String METADATA = """
        name: Example
        description: Example library
        groupId: example.library
        artifactId: example
        version: 1.2.3
        url: https://example.test
        licenses:
          - name: MIT License
            url: https://example.test/LICENSE
        developers:
          - id: example
            name: Example Developer
            email: developer@example.test
        scm:
          connection: scm:git:https://example.test/repo.git
          developerConnection: scm:git:ssh://example.test/repo.git
          url: https://example.test/repo
        """;

    private Path project(String metadata) throws Exception {
        Path root = temporary.newFolder().toPath();
        Files.writeString(root.resolve("settings.gradle"), "rootProject.name = 'example'\n");
        Files.writeString(root.resolve("build.gradle"), "plugins { id 'java-library'; id 'com.composables.kpm' }\n");
        if (metadata != null) Files.writeString(root.resolve("package.yml"), metadata);
        return root;
    }

    private GradleRunner runner(Path project, String... arguments) {
        return GradleRunner.create().withProjectDir(project.toFile()).withPluginClasspath().withArguments(arguments);
    }

    @Test public void configuresRealPublicationFromMetadataAndTracksVersionChanges() throws Exception {
        Path root = project(METADATA);
        runner(root, "generatePomFileForMavenPublication", "--configuration-cache").build();
        String pom = Files.readString(root.resolve("build/publications/maven/pom-default.xml"));
        assertTrue(pom.contains("<groupId>example.library</groupId>"));
        assertTrue(pom.contains("<artifactId>example</artifactId>"));
        assertTrue(pom.contains("<version>1.2.3</version>"));
        assertTrue(pom.contains("<name>MIT License</name>"));
        assertTrue(pom.contains("<email>developer@example.test</email>"));
        var cached = runner(root, "generatePomFileForMavenPublication", "--configuration-cache").build();
        assertTrue(cached.getOutput().contains("Reusing configuration cache"));
        Files.writeString(root.resolve("package.yml"), METADATA.replace("1.2.3", "1.2.4"));
        runner(root, "generatePomFileForMavenPublication", "--configuration-cache").build();
        assertTrue(Files.readString(root.resolve("build/publications/maven/pom-default.xml")).contains("<version>1.2.4</version>"));
        // This inspects task configuration only; it never contacts Maven Central.
        assertTrue(runner(root, "publishAllPublicationsToMavenCentral", "--dry-run").build().getOutput().contains("SKIPPED"));
    }

    private String nestedMetadata() {
        StringBuilder nested = new StringBuilder("name: Example\ndescription: Example library\nversion: 1.2.3\npublishing:\n");
        METADATA.lines().filter(line -> !line.startsWith("name:") && !line.startsWith("description:") && !line.startsWith("version:"))
            .forEach(line -> nested.append("  ").append(line).append('\n'));
        return nested.toString();
    }

    @Test public void nestedAndLegacyPublishingGenerateIdenticalPoms() throws Exception {
        Path legacy = project(METADATA);
        Path nested = project(nestedMetadata());
        var legacyResult = runner(legacy, "generatePomFileForMavenPublication").build();
        var nestedResult = runner(nested, "generatePomFileForMavenPublication").build();
        assertTrue(legacyResult.getOutput().contains("will be removed in 1.0.0"));
        assertFalse(nestedResult.getOutput().contains("top-level publishing metadata is deprecated"));
        assertEquals(Files.readString(legacy.resolve("build/publications/maven/pom-default.xml")),
            Files.readString(nested.resolve("build/publications/maven/pom-default.xml")));
    }

    @Test public void nestedPublishingTakesPrecedenceWithoutMergingLegacyFields() throws Exception {
        Path root = project(nestedMetadata() + "groupId: ignored.group\nartifactId: ignored-artifact\n");
        runner(root, "generatePomFileForMavenPublication").build();
        String pom = Files.readString(root.resolve("build/publications/maven/pom-default.xml"));
        assertTrue(pom.contains("<groupId>example.library</groupId>"));
        assertFalse(pom.contains("ignored"));
        assertTrue(runner(project(METADATA + "publishing: {}\n"), "help").buildAndFail().getOutput().contains("groupId"));
    }

    @Test public void allowsPackagesWithoutPublishing() throws Exception {
        Path root = project("name: Example\ndescription: Example app\nversion: 1.2.3\n");
        Files.writeString(root.resolve("build.gradle"), "plugins { id 'com.composables.kpm' }\n"
            + "assert project.version.toString() == '1.2.3'\n"
            + "assert !plugins.hasPlugin('com.vanniktech.maven.publish')\n");
        runner(root, "help", "--configuration-cache").build();
        assertTrue(runner(root, "help", "--configuration-cache").build().getOutput().contains("Reusing configuration cache"));
    }

    @Test public void rejectsMalformedOrIncompletePublishing() throws Exception {
        assertTrue(runner(project("name: Example\ndescription: Example\nversion: 1.2.3\npublishing: false\n"), "help")
            .buildAndFail().getOutput().contains("publishing must be a YAML mapping"));
        assertTrue(runner(project("name: Example\ndescription: Example\nversion: 1.2.3\ngroupId: example\n"), "help")
            .buildAndFail().getOutput().contains("artifactId"));
    }

    @Test public void rejectsMissingMetadata() throws Exception {
        assertTrue(runner(project(null), "help").buildAndFail().getOutput().contains("KPM requires package.yml"));
    }

    @Test public void rejectsDuplicateMetadataAndUnimplementedDependencies() throws Exception {
        assertTrue(runner(project(METADATA + "version: 2.0.0\n"), "help").buildAndFail().getOutput().contains("Invalid package.yml"));
        assertTrue(runner(project(METADATA + "dependencies: {}\n"), "help").buildAndFail().getOutput().contains("not supported yet"));
    }
}
