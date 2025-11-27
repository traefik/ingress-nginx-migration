# Ingress Nginx Analyzer

The Ingress Nginx Analyzer is a tool that analyzes Kubernetes Nginx Ingress resources to help with migration planning to Traefik.

## Features

- Analyzes all Ingress resources in a Kubernetes cluster
- Identifies nginx ingress controller annotations and their compatibility with Traefik
- Generates migration reports showing:
  - Total number of Ingress resources
  - How many can be migrated automatically
  - Which Ingress resources need manual attention
  - Unsupported annotations and their frequency
- Provides both local report generation and remote report submission

## Supported Nginx Annotations

The analyzer checks for compatibility with common nginx ingress controller annotations including:
- Authentication (`auth-type`, `auth-secret`, `auth-realm`, etc.)
- SSL/TLS (`force-ssl-redirect`, `ssl-redirect`, `ssl-passthrough`)
- Session affinity (`affinity`, `session-cookie-*`)
- Backend configuration (`service-upstream`, `backend-protocol`, `proxy-ssl-*`)
- CORS (`enable-cors`, `cors-allow-*`)
- And more...

## API Endpoints

| Method | Path           | Description                                        |
|--------|----------------|----------------------------------------------------|
| `GET`  | `/report`      | Generate and return migration analysis HTML report |
| `PUT`  | `/send-report` | Generate and send migration report to Traefik Labs |

## Usage

```console
NAME:
   Ingress Nginx Analyzer - Analyze Nginx Ingresses to build a migration report to Traefik

USAGE:
   Ingress Nginx Analyzer [global options]

OPTIONS:
   --addr string        server address (default: ":8080") [$ADDR]
   --log-level string   log level (default: "info") [$LOG_LEVEL]
   --kubeconfig string  path to kubeconfig file [$KUBECONFIG]
   --namespaces []string namespaces where to look for Ingresses [$NAMESPACES]
   --help, -h           show help
```

## Installation

```bash
go build -o ingress-nginx-analyzer ./cmd
```

## Running

Run with a kubeconfig file:

```bash
./ingress-nginx-analyzer --kubeconfig ~/.kube/config
```
