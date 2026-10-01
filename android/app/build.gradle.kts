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
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        versionCode = 21
        versionName = "1.1.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    testImplementation("junit:junit:4.13.2")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
    implementation("com.squareup.okhttp3:okhttp:5.5.0")
}
