package com.composables.kpm;

import com.vanniktech.maven.publish.DeploymentValidation;
import com.vanniktech.maven.publish.MavenPublishBaseExtension;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import org.gradle.api.GradleException;
import org.gradle.api.Plugin;
import org.gradle.api.Project;

/** Connect package metadata to Gradle without owning targets. */
public final class KpmPlugin implements Plugin<Project> {
    @Override
    public void apply(Project project) {
        var values = PackageManifest.read(project.getLayout().getProjectDirectory(), project.getProviders());
        if (values == null) {
            throw new GradleException("KPM requires package.yml next to this project's build.gradle(.kts).");
        }
        var metadata = new Metadata(values, project.getName());

        if (values.containsKey("plugins")) PackagePlugins.parse(values.get("plugins")).values().forEach(plugin ->
            project.getPluginManager().apply(plugin.id()));
        if (values.containsKey("dependencies") && values.get("dependencies") == null) {
            throw new GradleException("dependencies must be a list or a mapping of targets.");
        }
        new PackageDependencies(project, values.get("dependencies"));
        project.setVersion(metadata.version);
        if (!metadata.publishing) return;
        if (metadata.legacyPublishing) {
            project.getLogger().warn("KPM: top-level publishing metadata is deprecated and will be removed in 1.0.0; move it into a publishing block.");
        }
        project.setGroup(metadata.groupId);
        project.getPluginManager().apply("com.vanniktech.maven.publish");
        var publishing = project.getExtensions().getByType(MavenPublishBaseExtension.class);
        publishing.coordinates(metadata.groupId, metadata.artifactId, metadata.version);
        publishing.publishToMavenCentral(true, DeploymentValidation.NONE);
        if (!project.getProviders().gradleProperty("signingInMemoryKey").getOrElse("").isBlank()) {
            publishing.signAllPublications();
        }
        publishing.pom(pom -> {
            pom.getName().set(metadata.name);
            pom.getDescription().set(metadata.description);
            pom.getUrl().set(metadata.url);
            pom.licenses(licenses -> metadata.licenses.forEach(value -> licenses.license(license -> {
                license.getName().set(text(value, "name"));
                license.getUrl().set(text(value, "url"));
                if (value.containsKey("distribution")) license.getDistribution().set(text(value, "distribution"));
            })));
            pom.developers(developers -> metadata.developers.forEach(value -> developers.developer(developer -> {
                developer.getId().set(text(value, "id"));
                developer.getName().set(text(value, "name"));
                if (value.containsKey("url")) developer.getUrl().set(text(value, "url"));
                if (value.containsKey("email")) developer.getEmail().set(text(value, "email"));
            })));
            pom.scm(scm -> {
                scm.getConnection().set(text(metadata.scm, "connection"));
                scm.getDeveloperConnection().set(text(metadata.scm, "developerConnection"));
                scm.getUrl().set(text(metadata.scm, "url"));
            });
            if (metadata.issues != null) pom.issueManagement(issues -> {
                issues.getSystem().set(text(metadata.issues, "system"));
                issues.getUrl().set(text(metadata.issues, "url"));
            });
        });
    }

    private static Map<?, ?> mapping(Object value, String name) {
        if (!(value instanceof Map<?, ?> map)) throw new GradleException(name + " must be a YAML mapping.");
        return map;
    }

    private static String text(Map<?, ?> map, String key) {
        if (!(map.get(key) instanceof String value) || value.isBlank()) {
            throw new GradleException("package.yml requires a non-empty string for '" + key + "'.");
        }
        return value;
    }

    private static List<Map<?, ?>> entries(Map<?, ?> map, String key, String... required) {
        if (!(map.get(key) instanceof List<?> list) || list.isEmpty()) {
            throw new GradleException("package.yml requires a non-empty list for '" + key + "'.");
        }
        var entries = new ArrayList<Map<?, ?>>();
        for (Object item : list) {
            var entry = mapping(item, key);
            for (String field : required) text(entry, field);
            entries.add(entry);
        }
        return entries;
    }

    private static final class Metadata {
        final String groupId, artifactId, version, name, description, url;
        final List<Map<?, ?>> licenses, developers;
        final Map<?, ?> scm, issues;
        final boolean publishing, legacyPublishing;

        Metadata(Map<?, ?> values, String projectName) {
            version = text(values, "version");
            if (!version.matches("(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\\+[0-9A-Za-z.-]+)?")) {
                throw new GradleException("package.yml version must be a semantic version, such as 0.1.0.");
            }
            name = values.containsKey("name") ? text(values, "name") : projectName;
            description = values.containsKey("description") ? text(values, "description") : null;
            legacyPublishing = !values.containsKey("publishing") && List.of(
                "groupId", "artifactId", "url", "licenses", "developers", "scm", "issueManagement"
            ).stream().anyMatch(values::containsKey);
            publishing = values.containsKey("publishing") || legacyPublishing;
            if (!publishing) {
                groupId = artifactId = url = null;
                licenses = developers = List.of();
                scm = issues = null;
                return;
            }
            if (description == null) text(values, "description");
            var publication = legacyPublishing ? values : mapping(values.get("publishing"), "publishing");
            groupId = text(publication, "groupId");
            artifactId = text(publication, "artifactId");
            url = text(publication, "url");
            licenses = entries(publication, "licenses", "name", "url");
            developers = entries(publication, "developers", "id", "name");
            scm = mapping(publication.get("scm"), "scm");
            for (String key : List.of("connection", "developerConnection", "url")) text(scm, key);
            issues = publication.containsKey("issueManagement") ? mapping(publication.get("issueManagement"), "issueManagement") : null;
            if (issues != null) { text(issues, "system"); text(issues, "url"); }
        }
    }
}
