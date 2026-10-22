plugins {
    kotlin("jvm") version "1.9.24"
    antlr
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

dependencies {
    antlr("org.antlr:antlr4:4.13.2")
    implementation("org.antlr:antlr4-runtime:4.13.2")
    testImplementation(kotlin("test"))
}

// The antlr plugin puts the whole ANTLR tool on `api`; tacc needs only the runtime at run time.
configurations.api {
    setExtendsFrom(extendsFrom.filter { it.name != "antlr" })
}

// Generates TcLexer and TcParser from src/main/antlr/tacc/Tc.g4 into package `tacc`.
tasks.generateGrammarSource {
    arguments = arguments + listOf("-no-listener", "-no-visitor", "-package", "tacc")
}

// Kotlin code uses the generated Java classes, so generate them first.
tasks.compileKotlin {
    dependsOn(tasks.generateGrammarSource)
}

tasks.compileTestKotlin {
    dependsOn(tasks.generateTestGrammarSource)
}

tasks.test {
    useJUnitPlatform()
}
