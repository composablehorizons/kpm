plugins {
    `java-gradle-plugin`
    id("com.vanniktech.maven.publish") version "0.37.0"
}

group = "com.composables"
val packageMetadata = groovy.json.JsonSlurper().parseText(
    providers.fileContents(rootProject.layout.projectDirectory.file("package.json")).asText.get()
) as Map<*, *>
version = requireNotNull(packageMetadata["version"]) { "package.json must define version" }.toString()
repositories { mavenCentral(); gradlePluginPortal() }
java { toolchain { languageVersion.set(JavaLanguageVersion.of(17)) } }
gradlePlugin {
    plugins {
        create("kpm") {
            id = "com.composables.kpm"
            implementationClass = "com.composables.kpm.KpmPlugin"
            displayName = "Kotlin Package Manager"
            description = "Read package.yml and configure Kotlin library publication."
        }
    }
}
dependencies {
    implementation("com.vanniktech:gradle-maven-publish-plugin:0.37.0")
    implementation("org.yaml:snakeyaml:2.4")
    testImplementation(gradleTestKit())
    testImplementation("junit:junit:4.13.2")
}
tasks.test { maxParallelForks = 1 }

mavenPublishing {
    coordinates("com.composables", "kpm", project.version.toString())
    publishToMavenCentral(automaticRelease = true)
    if (providers.gradleProperty("signingInMemoryKey").orNull?.isNotBlank() == true) {
        signAllPublications()
    }
    pom {
        name.set("KPM")
        description.set("Package versioning and Maven Central publishing for Kotlin libraries.")
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
