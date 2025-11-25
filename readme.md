# Formica Exsecta: Server to manage update and collect from Traefik Proxy

Exsecta is an HTTP server used to analyze and push data to elastic search.

Exsecta handles:

- `update.traefik.io`
- `collect.traefik.io` (static v1, v2 and v3)
- `gateway.pilot.traefik.io` (Dynamic configuration v2)
- `assets.hub.traefik.io` (Traefik Hub assets Hub ui demo)

Exposed routes:

| Method | Path                                       | Description                           |
|--------|--------------------------------------------|---------------------------------------|
| `POST` | `/619df80498b60f985d766ce62f912b7c`        | Static v1 configuration               |
| `POST` | `/9vxmmkcdmalbdi635d4jgc5p5rx0h7h8`        | Static v2 configuration               |
| `POST` | `/yYaUej3P42cziRVzv6T5w2aYy9po2Mrn`        | Static v3 configuration               |
| `POST` | `/1e156859efd05531b2cf77a48ef887dd`        | Traefik Hub static configuration      |
| `POST` | `/collect`                                 | Dynamic v2 configuration (Deprecated) |
| `GET`  | `/repos/traefik/traefik/releases`          | Traefik update                        |
| `GET`  | `/repos/containous/traefik/releases`       | Legacy Traefik update                 |
| `GET`  | `/repos/traefik/hub-agent-traefik/tags`    | Hub agent Traefik update              |
| `GET`  | `/repos/traefik/hub-agent-kubernetes/tags` | Hub agent Kubernetes update           |
| `GET`  | `/traefik-hub/latest-version`              | Traefik Hub latest version            |
| `GET`  | `/hub-demo-app.js`                         | Hub demo app JavaScript               |
| `GET`  | `/hub-demo-app.js.sig`                     | Hub demo app JavaScript               |

```console
NAME:
   Formica Exsecta - Server to manage update and collect from Traefik Proxy

USAGE:
   Formica Exsecta [global options] [command [command options]]

COMMANDS:
   serve    Serve HTTP
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help
```

- Serve command
```console
NAME:
   Formica Exsecta serve - Serve HTTP

USAGE:
   Formica Exsecta serve [options]

DESCRIPTION:
   Launch application exsecta service

OPTIONS:
   --host string                        host (default: ":8080") [$HOST]
   --log-level string                   log-level (default: "info") [$LOG_LEVEL]
   --es-cloud-id string                 es-cloud-id [$ES_CLOUD_ID]
   --es-api-key string                  es-api-key [$ES_API_KEY]
   --es-username string                 es-username [$ES_USERNAME]
   --es-password string                 es-password [$ES_PASSWORD]
   --es-workers int                     es-workers (default: 5) [$ES_WORKERS]
   --es-flush-interval duration         es-flush-interval (default: 30s) [$ES_FLUSH_INTERVAL]
   --workers int                        workers (default: 1000) [$WORKERS]
   --dry-run                            dry-run [$DRY_RUN]
   --traefik-hub-latest-version string  traefik-hub-latest-version (default: "v2.0.0") [$TRAEFIK_HUB_LATEST_VERSION]
   --hub-ui-demo-assets-url string      hub-ui-demo-assets-url (default: "https://traefik.github.io/hub-ui-demo-app/scripts/hub-ui-demo.umd.js") [$HUB_UI_DEMO_ASSETS_URL]
   --help, -h                           show help
```

## What does Formica exsecta mean?

![Formica exsecta](https://www.antwiki.org/wiki/images/thumb/b/bf/Formica_exsecta_casent0173161_head_1.jpg/400px-Formica_exsecta_casent0173161_head_1.jpg)
