plugins {
    id("com.android.application")
}

android {
    namespace = "com.arjanflac.tailboard"
    compileSdk = 37

    defaultConfig {
        applicationId = "com.arjanflac.tailboard"
        minSdk = 29
        targetSdk = 37
        versionCode = 18
        versionName = "1.0.1"
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
