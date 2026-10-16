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

    @Test public void rejectsMissingMetadata() throws Exception {
        assertTrue(runner(project(null), "help").buildAndFail().getOutput().contains("KPM requires package.yml"));
    }

    @Test public void rejectsDuplicateMetadataAndUnimplementedDependencies() throws Exception {
        assertTrue(runner(project(METADATA + "version: 2.0.0\n"), "help").buildAndFail().getOutput().contains("Invalid package.yml"));
        assertTrue(runner(project(METADATA + "dependencies: {}\n"), "help").buildAndFail().getOutput().contains("not supported yet"));
    }
}
