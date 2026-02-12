FROM eclipse-temurin:16-jdk AS jre-builder

RUN $JAVA_HOME/bin/jlink \
  --add-modules java.base,java.logging,java.management,java.naming,java.security.jgss,java.instrument,java.sql,java.xml,java.desktop,jdk.unsupported,jdk.crypto.ec,jdk.zipfs,jdk.security.auth \
  --strip-debug \
  --no-man-pages \
  --no-header-files \
  --compress=2 \
  --output /opt/java-min

FROM debian:bookworm-slim AS runtime

WORKDIR /data/server

COPY --from=jre-builder /opt/java-min /opt/java-min
COPY . .

RUN chmod +x /data/server/run.sh \
  && mkdir -p /data/server/world /data/server/world_nether /data/server/world_the_end

ENV JAVA_BIN=/opt/java-min/bin/java \
    MEMORY_MIN=1G \
    MEMORY_MAX=2G \
    PAPER_JAR=paper-1.16.5-794.jar \
    EXTRA_JAVA_FLAGS="-XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32" \
    NOGUI=true

EXPOSE 25565 4567
VOLUME ["/data/server/world", "/data/server/world_nether", "/data/server/world_the_end"]

ENTRYPOINT ["/data/server/run.sh"]
