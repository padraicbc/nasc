import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "app.nasc"
    compileSdk = 36

    defaultConfig {
        applicationId = "app.nasc"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "1.0.0"
    }

    // ELF alignment (16 KB pages) is set by the gomobile linker flags in scripts/build-android.sh.
    packaging { jniLibs.useLegacyPackaging = false }

    // Play wants the bundle's native-symbol info so crashes in the Go runtime are readable.
    buildTypes.getByName("release").ndk.debugSymbolLevel = "SYMBOL_TABLE"

    // Signing comes from android/keystore.properties (Play upload key) or the file named by
    // -Pnasc.keystore (both gitignored; see RELEASE.md).
    // Without it the release bundle is built unsigned, which Play will refuse -- on purpose, so a
    // debug-signed build can never be uploaded by accident.
    // -Pnasc.keystore=<file> picks another properties file (scripts/release-apk.sh uses
    // keystore-github.properties so the GitHub APK and the Play upload key stay separate).
    val keystoreFile = (findProperty("nasc.keystore") as String?) ?: "keystore.properties"
    val keystoreProps = Properties().apply {
        rootProject.file(keystoreFile).takeIf { it.exists() }?.inputStream()?.use { load(it) }
    }
    signingConfigs {
        if (keystoreProps.isNotEmpty()) {
            create("upload") {
                storeFile = rootProject.file(keystoreProps.getProperty("storeFile"))
                storePassword = keystoreProps.getProperty("storePassword")
                keyAlias = keystoreProps.getProperty("keyAlias")
                keyPassword = keystoreProps.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = when {
                keystoreProps.isNotEmpty() -> signingConfigs.getByName("upload")
                // Local smoke-testing only: ./gradlew assembleRelease -Pnasc.debugSign=true
                project.hasProperty("nasc.debugSign") -> signingConfigs.getByName("debug")
                else -> null
            }
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

kotlin { compilerOptions { jvmTarget.set(JvmTarget.JVM_17) } }

dependencies {
    // Built by scripts/build-android.sh (gomobile bind): SSH core + embedded web app.
    implementation(files("libs/nasc.aar"))
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.biometric:biometric:1.1.0")
}
