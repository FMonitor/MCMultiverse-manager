# MiniMap Runtime Images

This folder contains three runtime targets:

- `MiniMap-Java16` for MC 1.16.x
- `MiniMap-Java17` for MC 1.17.x - 1.20.4
- `MiniMap-Java21` for MC 1.20.5+ / 1.21.x

Each target has:

- `Dockerfile`
- `run.sh` (env-driven startup)
- bundled plugins (`ServerTap`, `mcmmrequester`)

## Build

```bash
docker build -t mcmm-mini:java16 deploy/runtime/MiniMap-Java16
docker build -t mcmm-mini:java17 deploy/runtime/MiniMap-Java17
docker build -t mcmm-mini:java21 deploy/runtime/MiniMap-Java21
```

## Run (mount world + core only)

```bash
docker run -d --name mcmm-mini-1211 \
  -e MEMORY_MIN=1G \
  -e MEMORY_MAX=4G \
  -e PAPER_JAR=paper-1.21.1-123.jar \
  -e NOGUI=true \
  -p 25565:25565 \
  -p 4567:4567 \
  -v /path/to/core/paper-1.21.1-123.jar:/data/server/paper-1.21.1-123.jar:ro \
  -v /path/to/world:/data/server/world \
  -v /path/to/world_nether:/data/server/world_nether \
  -v /path/to/world_the_end:/data/server/world_the_end \
  mcmm-mini:java21
```

## Runtime environment variables

- `JAVA_BIN` (default: `java`)
- `MEMORY_MIN` (default: `1G`)
- `MEMORY_MAX` (default: `2G`)
- `PAPER_JAR` (default is version-specific in each runtime folder)
- `NOGUI` (`true`/`false`, default: `true`)
- `EXTRA_JAVA_FLAGS` (optional JVM flags override)

