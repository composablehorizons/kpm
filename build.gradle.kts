import com.vanniktech.maven.publish.DeploymentValidation
import com.vanniktech.maven.publish.MavenPublishBaseExtension

plugins {
    base
    id("com.vanniktech.maven.publish") version "0.37.0" apply false
}

version = providers.fileContents(layout.projectDirectory.file("VERSION")).asText.get().trim()
subprojects {
    group = "com.composables"
    version = rootProject.version
    pluginManager.withPlugin("com.vanniktech.maven.publish") {
        extensions.configure<MavenPublishBaseExtension> {
            coordinates("com.composables", if (project.name == "gradle-plugin") "kpm" else "kpm-settings", project.version.toString())
            publishToMavenCentral(automaticRelease = true, validateDeployment = DeploymentValidation.NONE)
            if (providers.gradleProperty("signingInMemoryKey").orNull?.isNotBlank() == true) signAllPublications()
            pom {
                name.set(if (project.name == "gradle-plugin") "KPM" else "KPM settings integration")
                description.set("Dependency management, versioning, and Maven Central publishing for Kotlin packages.")
                url.set("https://github.com/composablehorizons/kpm")
                licenses {
                    license {
                        name.set("MIT License")
                        url.set("https://opensource.org/license/mit")
                        distribution.set("repo")
                    }
                }
                developers {
                    developer {
                        id.set("composablehorizons")
                        name.set("Composable Horizons")
                        url.set("https://composables.com")
                    }
                }
                scm {
                    connection.set("scm:git:https://github.com/composablehorizons/kpm.git")
                    developerConnection.set("scm:git:ssh://git@github.com/composablehorizons/kpm.git")
                    url.set("https://github.com/composablehorizons/kpm")
                }
            }
        }
    }
}
