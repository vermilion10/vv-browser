import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "dev.vvbrowser"
    compileSdk = 36

    defaultConfig {
        applicationId = "dev.vvbrowser"
        minSdk = 33
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"
    }

    // Release signing is read from the user's Gradle properties
    // (~/.gradle/gradle.properties), never from the repository. Without them,
    // release builds are left unsigned.
    val storeFile = providers.gradleProperty("VV_RELEASE_STORE_FILE").orNull
    signingConfigs {
        if (storeFile != null) {
            create("release") {
                this.storeFile = file(storeFile)
                storePassword = providers.gradleProperty("VV_RELEASE_STORE_PASSWORD").get()
                keyAlias = providers.gradleProperty("VV_RELEASE_KEY_ALIAS").get()
                keyPassword = providers.gradleProperty("VV_RELEASE_KEY_PASSWORD").get()
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.findByName("release")
            // Phones and tablets only; the x86_64 core is for the emulator.
            ndk { abiFilters += "arm64-v8a" }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

dependencies {
    // Network core, built from ../core with scripts/build-core.sh
    implementation(files("libs/vvcore.aar"))
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.activity:activity-ktx:1.11.0")
    implementation("androidx.webkit:webkit:1.14.0")
}
