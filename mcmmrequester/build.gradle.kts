plugins {
    java
}

repositories {
    maven("https://maven.aliyun.com/repository/public")
    mavenCentral()
    maven("https://repo.papermc.io/repository/maven-public/")
}

dependencies {
    compileOnly("com.destroystokyo.paper:paper-api:1.16.5-R0.1-SNAPSHOT")
}

java {
    toolchain {
        languageVersion.set(JavaLanguageVersion.of(21))
    }
}

tasks {
    processResources {
        filteringCharset = "UTF-8"
    }

    withType<JavaCompile> {
        options.encoding = "UTF-8"
        options.release.set(8)
    }

    jar {
        archiveBaseName.set("mcmmrequester")
    }
}
