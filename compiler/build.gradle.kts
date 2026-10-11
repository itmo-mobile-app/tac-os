plugins {
    kotlin("jvm") version "1.9.24"
    application
}

group = "tac-os"
version = "0.1.0"

repositories {
    mavenCentral()
}

application {
    mainClass.set("tacc.MainKt")
}

kotlin {
    jvmToolchain(17)
}

tasks.test {
    useJUnitPlatform()
}

dependencies {
    testImplementation(kotlin("test"))
}
