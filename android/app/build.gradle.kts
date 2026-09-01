plugins {
    id("com.android.application")
}

android {
    namespace = "com.arjanflac.tgclipboard"
    compileSdk = 37

    defaultConfig {
        applicationId = "com.arjanflac.tgclipboard"
        minSdk = 29
        targetSdk = 37
        versionCode = 16
        versionName = "0.9.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    implementation("com.squareup.okhttp3:okhttp:5.5.0")
    testImplementation("junit:junit:4.13.2")
}
