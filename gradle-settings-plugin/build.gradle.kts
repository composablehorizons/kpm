plugins {
    `java-gradle-plugin`
    id("com.vanniktech.maven.publish")
}

repositories { mavenCentral() }
java { toolchain { languageVersion.set(JavaLanguageVersion.of(17)) } }
gradlePlugin {
    plugins {
        create("kpmSettings") {
            id = "com.composables.kpm.settings"
            implementationClass = "com.composables.kpm.KpmSettingsPlugin"
            displayName = "Kotlin Package Manager plugin declarations"
            description = "Resolve package.yml plugins before project scripts compile."
        }
    }
}
dependencies { implementation("org.yaml:snakeyaml:2.4") }
tasks.processResources {
    inputs.property("version", project.version.toString())
    filesMatching("kpm-version.properties") { expand("version" to project.version.toString()) }
}
