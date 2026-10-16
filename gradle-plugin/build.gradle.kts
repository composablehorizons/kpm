plugins {
    `java-gradle-plugin`
    `maven-publish`
}

group = "com.composables.kpm"
version = "0.1.0-SNAPSHOT"
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
