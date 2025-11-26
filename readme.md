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

| Method | Path           | Description                                          |
|--------|----------------|------------------------------------------------------|
| `GET`  | `/report`      | Generate and return migration analysis report (JSON) |
| `PUT`  | `/send-report` | Generate and send migration report to Traefik Labs   |

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
   --help, -h           show help
```

## Installation

```bash
go build -o ingress-nginx-analyzer ./cmd
```

## Running

### In-cluster (recommended)
Deploy the analyzer as a pod in your Kubernetes cluster where it can automatically discover and analyze Ingress resources:

```bash
./ingress-nginx-analyzer
```

### Local development
Run locally with a kubeconfig file:

```bash
./ingress-nginx-analyzer --kubeconfig ~/.kube/config
```

## Example Report

The analyzer generates reports in JSON format showing migration compatibility:

```json
{
  "IngressCount": 100,
  "CompatibleIngressCount": 80,
  "UnsupportedIngressCount": 20,
  "UnsupportedIngressAnnotations": {
    "nginx.ingress.kubernetes.io/custom-annotation": 5,
    "nginx.ingress.kubernetes.io/another-unsupported": 3
  },
  "UnsupportedIngresses": [
    {
      "Name": "my-app-ingress",
      "Namespace": "default",
      "IngressClassName": "nginx",
      "UnsupportedAnnotations": ["nginx.ingress.kubernetes.io/custom-annotation"]
    }
  ]
}
```
