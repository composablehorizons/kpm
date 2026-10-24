pluginManagement {
    includeBuild("../../")
    repositories { gradlePluginPortal(); mavenCentral() }
}
plugins { id("com.composables.kpm.settings") }
rootProject.name = "example-library"
