plugins {
    kotlin("multiplatform") version "2.4.0"
    id("com.composables.kpm")
}
repositories { mavenCentral() }
kotlin {
    jvmToolchain(17)
    jvm()
    @OptIn(org.jetbrains.kotlin.gradle.ExperimentalWasmDsl::class)
    wasmJs { browser() }
}
