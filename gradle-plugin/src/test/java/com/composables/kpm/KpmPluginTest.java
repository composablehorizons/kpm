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

    private void localArtifact(Path repository, String version) throws Exception {
        Path directory = repository.resolve("example/tool/" + version);
        Files.createDirectories(directory);
        Files.writeString(directory.resolve("tool-" + version + ".pom"),
            "<project><modelVersion>4.0.0</modelVersion><groupId>example</groupId><artifactId>tool</artifactId><version>" + version + "</version></project>");
        try (var jar = new java.util.jar.JarOutputStream(Files.newOutputStream(directory.resolve("tool-" + version + ".jar")))) {}
    }

    @Test public void resolvesInstallsListsAndLooksUpVersionsFromProjectRepositories() throws Exception {
        Path root = project("name: Example\ndescription: Example app\nversion: 1.2.3\ndependencies:\n  - example:tool:1.0.0\n");
        Path repo = root.resolve("repo");
        localArtifact(repo, "1.0.0"); localArtifact(repo, "1.1.0");
        Files.writeString(repo.resolve("example/tool/maven-metadata.xml"),
            "<metadata><groupId>example</groupId><artifactId>tool</artifactId><versioning><latest>1.1.0</latest><release>1.1.0</release><versions><version>1.0.0</version><version>1.1.0</version></versions></versioning></metadata>");
        Files.writeString(root.resolve("build.gradle"), "plugins { id 'com.composables.kpm'; id 'java-library' }\nrepositories { maven { url = uri('repo') } }\n");
        Path report = root.resolve("report.json");
        runner(root, "kpmInstall", "--no-configuration-cache", "-Pkpm.report=" + report).build();
        assertTrue(Files.readString(report).contains("\"current\":\"1.0.0\""));
        runner(root, "kpmList", "--no-configuration-cache", "-Pkpm.target=jvm", "-Pkpm.report=" + report).build();
        assertTrue(Files.readString(report).contains("example:tool"));
        runner(root, "kpmOutdated", "--no-configuration-cache", "-Pkpm.report=" + report).build();
        String outdated = Files.readString(report);
        assertTrue(outdated.contains("\"latest\":\"1.1.0\""));
        assertTrue(outdated.contains("\"wanted\":\"1.0.0\""));
        runner(root, "kpmLookup", "--no-configuration-cache", "-Pkpm.lookup=example:tool:latest.release", "-Pkpm.report=" + report).build();
        assertTrue(Files.readString(report).contains("\"version\":\"1.1.0\""));
        Files.writeString(root.resolve("package.yml"), Files.readString(root.resolve("package.yml")).replace("example:tool:1.0.0", "example:tool:1.+"));
        runner(root, "kpmInstall", "--refresh-dependencies", "--no-configuration-cache", "-Pkpm.report=" + report).build();
        assertTrue(Files.readString(report).contains("\"current\":\"1.1.0\""));
    }

    @Test public void resolvesVersionlessDependenciesUsingBomAndPreservesManagedWantedVersion() throws Exception {
        Path root = project("name: Example\ndescription: Example app\nversion: 1.2.3\ndependencies: [example:tool]\n");
        Path repo = root.resolve("repo");
        localArtifact(repo, "1.0.0"); localArtifact(repo, "1.1.0");
        Files.writeString(repo.resolve("example/tool/maven-metadata.xml"),
            "<metadata><groupId>example</groupId><artifactId>tool</artifactId><versioning><release>1.1.0</release><versions><version>1.0.0</version><version>1.1.0</version></versions></versioning></metadata>");
        Path bom = repo.resolve("example/alignment/1.0");
        Files.createDirectories(bom);
        Files.writeString(bom.resolve("alignment-1.0.pom"), """
            <project><modelVersion>4.0.0</modelVersion><groupId>example</groupId><artifactId>alignment</artifactId>
            <version>1.0</version><packaging>pom</packaging><dependencyManagement><dependencies>
            <dependency><groupId>example</groupId><artifactId>tool</artifactId><version>1.0.0</version></dependency>
            </dependencies></dependencyManagement></project>
            """);
        String build = "plugins { id 'com.composables.kpm'; id 'java-library' }\nrepositories { maven { url = uri('repo') } }\n";
        Files.writeString(root.resolve("build.gradle"), build + "dependencies { implementation platform('example:alignment:1.0') }\n");
        Path report = root.resolve("report.json");
        for (String task : new String[] {"kpmInstall", "kpmList", "kpmOutdated"}) {
            runner(root, task, "--no-configuration-cache", "-Pkpm.report=" + report).build();
            String result = Files.readString(report);
            assertTrue(result.contains("\"id\":\"example:tool\""));
            assertTrue(result.contains("\"declared\":\"\""));
            assertTrue(result.contains("\"current\":\"1.0.0\""));
            assertTrue(result.contains("\"wanted\":\"1.0.0\""));
            if (task.equals("kpmOutdated")) assertTrue(result.contains("\"latest\":\"1.1.0\""));
        }
        // Gradle must fail when no BOM or other constraint supplies the missing version.
        Files.writeString(root.resolve("build.gradle"), build);
        runner(root, "kpmInstall", "--no-configuration-cache", "-Pkpm.report=" + report).buildAndFail();
        Files.writeString(root.resolve("package.yml"), "name: Example\ndescription: Example\nversion: 1.2.3\ndependencies: [example:tool, example:tool:1.0.0]\n");
        assertTrue(runner(root, "help").buildAndFail().getOutput().contains("Duplicate dependency"));
    }

    @Test public void attachesExplicitScopesToGradleAndRejectsMissingTargets() throws Exception {
        Path root = project("name: Example\ndescription: Example\nversion: 1.2.3\ndependencies:\n  jvm:\n    api: [example:public:1.0]\n    implementation: [example:internal:1.0]\n");
        Files.writeString(root.resolve("build.gradle"), "plugins { id 'com.composables.kpm'; id 'java-library' }\nafterEvaluate { assert configurations.api.dependencies*.name == ['public']; assert configurations.implementation.dependencies*.name == ['internal'] }\n");
        runner(root, "help", "--configuration-cache").build();
        assertTrue(runner(root, "help", "--configuration-cache").build().getOutput().contains("Reusing configuration cache"));
        Files.writeString(root.resolve("package.yml"), Files.readString(root.resolve("package.yml")).replace("jvm:", "android:"));
        assertTrue(runner(root, "help").buildAndFail().getOutput().contains("does not exist"));
    }

    @Test public void configuresRealKotlinMultiplatformSourceSets() throws Exception {
        Path root = project("name: Example\ndescription: Example\nversion: 1.2.3\ndependencies:\n  common:\n    api: [example:public:1.0]\n  jvm: [example:internal:1.0]\n");
        Files.writeString(root.resolve("settings.gradle"), "pluginManagement { repositories { gradlePluginPortal(); mavenCentral() } }\nrootProject.name = 'example'\n");
        Files.writeString(root.resolve("build.gradle"),
            "plugins { id 'com.composables.kpm'; id 'org.jetbrains.kotlin.multiplatform' version '2.4.0' }\n"
            + "kotlin { jvm() }\n"
            + "afterEvaluate { assert configurations.commonMainApi.dependencies*.name.contains('public'); assert configurations.jvmMainImplementation.dependencies*.name.contains('internal') }\n");
        runner(root, "help", "--configuration-cache").build();
        assertTrue(runner(root, "help", "--configuration-cache").build().getOutput().contains("Reusing configuration cache"));
        Files.writeString(root.resolve("package.yml"), "name: Example\ndescription: Example\nversion: 1.2.3\ndependencies: [example:tool:1.0.0]\n");
        Files.writeString(root.resolve("build.gradle"), "plugins { id 'org.jetbrains.kotlin.multiplatform' version '2.4.0'; id 'com.composables.kpm' }\nkotlin { jvm() }\nafterEvaluate { assert configurations.commonMainImplementation.dependencies*.name.contains('tool') }\n");
        runner(root, "help").build();
        Files.writeString(root.resolve("package.yml"), "name: Example\ndescription: Example\nversion: 1.2.3\ndependencies: [org.jetbrains.kotlin:kotlin-stdlib:2.4.0]\n");
        Files.writeString(root.resolve("build.gradle"), "plugins { id 'org.jetbrains.kotlin.multiplatform' version '2.4.0'; id 'com.composables.kpm' }\nrepositories { mavenCentral() }\nkotlin { jvm() }\n");
        Path report = root.resolve("report.json");
        runner(root, "kpmInstall", "--no-configuration-cache", "-Pkpm.report=" + report).build();
        assertTrue(Files.readString(report).contains("\"current\":\"2.4.0\""));
    }

    @Test public void rejectsMissingMetadata() throws Exception {
        assertTrue(runner(project(null), "help").buildAndFail().getOutput().contains("KPM requires package.yml"));
    }

    @Test public void rejectsDuplicateMetadataAndInvalidDependencies() throws Exception {
        assertTrue(runner(project(METADATA + "version: 2.0.0\n"), "help").buildAndFail().getOutput().contains("Invalid package.yml"));
        assertTrue(runner(project(METADATA + "dependencies:\n  common:\n    runtimeOnly: []\n"), "help").buildAndFail().getOutput().contains("Unsupported dependency scope"));
    }
}
