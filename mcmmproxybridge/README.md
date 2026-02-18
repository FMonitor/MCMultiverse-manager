# MCMMProxyBridge (Velocity Plugin)

This plugin exposes a small HTTP API for dynamic proxy server registration.

## Build

```bash
gradle -p mcmmproxybridge build
```

Jar output:

```text
mcmmproxybridge/build/libs/mcmmproxybridge-0.1.0.jar
```

## Install

1. Copy jar to Velocity `plugins/`.
2. Start proxy once to generate `plugins/mcmmproxybridge/config.properties`.
3. Edit `auth_token`.
4. Restart proxy.

## Config

`config.properties`

```properties
listen_host=0.0.0.0
listen_port=19132
auth_header=Authorization
auth_token=replace-with-real-token
```

## API

Auth: header `Authorization: Bearer <token>` (or raw token value).

- `POST /v1/proxy/register`
  - form: `server_id`, `host`, `port`
- `POST /v1/proxy/unregister`
  - form: `server_id`
- `POST /v1/proxy/send`
  - form: `player`, `server_id`
- `GET /v1/proxy/servers`
- `GET /v1/proxy/players?server_id=<id>`
