plugins {
    id("com.composables.kpm")
}
repositories { mavenCentral() }
kotlin {
    jvmToolchain(17)
    jvm()
    @OptIn(org.jetbrains.kotlin.gradle.ExperimentalWasmDsl::class)
    wasmJs { browser() }
}
