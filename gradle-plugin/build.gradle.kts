plugins {
    `java-gradle-plugin`
    id("com.vanniktech.maven.publish")
}

repositories { mavenCentral(); gradlePluginPortal() }
java { toolchain { languageVersion.set(JavaLanguageVersion.of(17)) } }
gradlePlugin {
    plugins {
        create("kpm") {
            id = "com.composables.kpm"
            implementationClass = "com.composables.kpm.KpmPlugin"
            displayName = "Kotlin Package Manager"
            description = "Read package.yml to manage dependencies and configure Kotlin library publication."
        }
    }
}
dependencies {
    implementation("com.vanniktech:gradle-maven-publish-plugin:0.37.0")
    implementation(project(":gradle-settings-plugin"))
    testImplementation(gradleTestKit())
    testImplementation("junit:junit:4.13.2")
}
tasks.test { maxParallelForks = 1 }
