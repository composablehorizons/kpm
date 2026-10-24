package com.composables.kpm;

import org.gradle.api.Plugin;
import org.gradle.api.Project;

/** Standalone plugin packaged into the tests' local Maven repository. */
public final class ExamplePlugin implements Plugin<Project> {
    @Override public void apply(Project project) {
        project.getPluginManager().apply("java-library");
    }
}
