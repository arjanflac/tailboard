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
        versionCode = 20
        versionName = "1.0.3"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    testImplementation("junit:junit:4.13.2")
    implementation("com.squareup.okhttp3:okhttp:5.5.0")
}
