#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

JAVA_BIN="${JAVA_BIN:-java}"
JAR="velocity.jar"
RAM="${RAM:-4g}"
FLAGS="-XX:+UseG1GC -XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=200 -XX:+UnlockExperimentalVMOptions -XX:+DisableExplicitGC -XX:+AlwaysPreTouch -XX:G1NewSizePercent=30 -XX:G1MaxNewSizePercent=40 -XX:G1HeapRegionSize=8M -XX:G1ReservePercent=20 -XX:G1HeapWastePercent=5 -XX:G1MixedGCCountTarget=4 -XX:InitiatingHeapOccupancyPercent=15 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:SurvivorRatio=32 -XX:+PerfDisableSharedMem -XX:MaxTenuringThreshold=1 -Daikars.new.flags=true -Dusing.aikars.flags=https://mcflags.emc.gs"

echo "Starting server..."
exec "$JAVA_BIN" -Xmx"$RAM" -Xms"$RAM" $FLAGS -Dfile.encoding=UTF-8 -javaagent:authlib-injector-1.2.5.jar=littleskin.cn -jar "$JAR"
