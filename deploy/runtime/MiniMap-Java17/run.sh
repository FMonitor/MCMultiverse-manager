#!/bin/sh
set -eu

JAVA_BIN="${JAVA_BIN:-java}"
MEMORY_MIN="${MEMORY_MIN:-1G}"
MEMORY_MAX="${MEMORY_MAX:-2G}"
PAPER_JAR="${PAPER_JAR:-paper-1.18.2-388.jar}"
NOGUI="${NOGUI:-true}"
EXTRA_JAVA_FLAGS="${EXTRA_JAVA_FLAGS:--XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32}"

if [ ! -f "${PAPER_JAR}" ]; then
  candidate="$(ls -1 paper-*.jar server.jar 2>/dev/null | head -n 1 || true)"
  if [ -n "${candidate}" ]; then
    PAPER_JAR="${candidate}"
  else
    echo "[run.sh] ERROR: core jar not found. Set PAPER_JAR or mount core jar into /data/server." >&2
    exit 1
  fi
fi

if ! command -v "${JAVA_BIN}" >/dev/null 2>&1; then
  echo "[run.sh] ERROR: JAVA_BIN '${JAVA_BIN}' not found in PATH." >&2
  exit 1
fi

set -- "${JAVA_BIN}" "-Xms${MEMORY_MIN}" "-Xmx${MEMORY_MAX}"
if [ -n "${EXTRA_JAVA_FLAGS}" ]; then
  # shellcheck disable=SC2086
  set -- "$@" ${EXTRA_JAVA_FLAGS}
fi
set -- "$@" -jar "${PAPER_JAR}"

nogui_normalized="$(printf '%s' "${NOGUI}" | tr '[:upper:]' '[:lower:]')"
if [ "${nogui_normalized}" = "true" ] || [ "${NOGUI}" = "1" ]; then
  set -- "$@" nogui
fi

echo "[run.sh] starting with JAVA_BIN=${JAVA_BIN}, Xms=${MEMORY_MIN}, Xmx=${MEMORY_MAX}, PAPER_JAR=${PAPER_JAR}, NOGUI=${NOGUI}"
exec "$@"
