
# Ingress NGINX Migration

<img width="1185" height="816" alt="screenshot black" src="https://github.com/user-attachments/assets/e2d62f62-4dee-49ab-9012-5decc1bda0f0" />

The Ingress NGINX Migration is a tool that analyzes Kubernetes NGINX Ingress resources to help with migration planning to Traefik.

## Features

The Ingress NGINX Migration tool creates and serves an interactive HTML report,
and to do so it:

- Analyzes all Ingress resources in a Kubernetes cluster or specific namespaces
- Identifies Ingress NGINX Controller annotations and their compatibility with Traefik
- Supports both in-cluster deployment and external kubeconfig access
- Generates timestamped migration HTML report showing:
  - Total number of Ingress resources
  - How many can be migrated automatically
  - Which Ingress resources need manual attention
  - Unsupported annotations and their frequency
- Provides flexible ingress filtering by controller class, ingress class name, and namespace

## Supported NGINX Annotations

The Ingress NGINX Migration checks for compatibility with common Ingress NGINX Controller annotations including:
- Authentication (`auth-type`, `auth-secret`, `auth-realm`, etc.)
- SSL/TLS (`force-ssl-redirect`, `ssl-redirect`, `ssl-passthrough`)
- Session affinity (`affinity`, `session-cookie-*`)
- Backend configuration (`service-upstream`, `backend-protocol`, `proxy-ssl-*`)
- CORS (`enable-cors`, `cors-allow-*`)
- And more...

For a complete list of supported annotations and their Traefik equivalents, see the [Ingress NGINX Annotations table](https://doc.traefik.io/traefik/reference/routing-configuration/kubernetes/ingress-nginx/#annotations-support) in the Traefik documentation.

## Installation

### Quick Install (Recommended)

Install the latest version using the install script:

```bash
curl -sSL https://raw.githubusercontent.com/traefik/ingress-nginx-migration/main/scripts/install.sh | bash
```

Install a specific version:

```bash
curl -sSL https://raw.githubusercontent.com/traefik/ingress-nginx-migration/main/scripts/install.sh | TAG=v0.0.1 bash
```

Install without sudo (installs to `~/bin`):

```bash
curl -sSL https://raw.githubusercontent.com/traefik/ingress-nginx-migration/main/scripts/install.sh | bash -s -- --no-sudo
```

### Manual Download

Download the appropriate binary for your platform from the [releases page](https://github.com/traefik/ingress-nginx-migration/releases).

## Usage

```console
NAME:
   ingress-nginx-migration - Analyze NGINX Ingresses to build a migration report to Traefik

USAGE:
   ingress-nginx-migration [global options] [command [command options]]

COMMANDS:
   version  Shows the current version
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --addr string                                  Defines the address to listen on for serving the migration report. (default: ":8080") [$ADDR]
   --kubeconfig string                            Defines the kubeconfig file to use to connect to the Kubernetes cluster. [$KUBECONFIG]
   --namespaces string [ --namespaces string ]    Defines the namespaces to analyze. When empty, all namespaces are analyzed. [$NAMESPACES]
   --ingress-class string                         Defines the name of the ingress class this controller satisfies. [$INGRESS_CLASS]
   --controller-class string                      Defines the Ingress Controller class to analyze. When empty, 'k8s.io/ingress-nginx' is used. [$CONTROLLER_CLASS]
   --watch-ingress-without-class                  Defines if Ingress Controller should also watch for Ingresses without an IngressClass or the annotation specified. [$WATCH_INGRESS_WITHOUT_CLASS]
   --ingress-class-by-name                        Defines if Ingress Controller should watch for Ingress Class by Name together with Controller Class. [$INGRESS_CLASS_BY_NAME]
   --help, -h                                     Show help
```

> [!TIP]
> **Quick Start:** Run the tool with your kubeconfig:
> ```bash
> ingress-nginx-migration --kubeconfig ~/.kube/config
> ```
> Or using environment variables:
> ```bash
> KUBECONFIG=~/.kube/config ingress-nginx-migration
> ```

### Required Permissions

The Ingress NGINX Migration requires specific read-only permissions to analyze your cluster's Ingress resources.
The tool is principally designed to be run externally by SREs and system administrators who have access to a Kubernetes cluster via kubeconfig.
But the same requirement would apply for an In-Cluster pod approach.

Your kubeconfig user or service account must have the following permissions:

| API Group              | Resources        | Verbs                  | Scope             |
|------------------------|------------------|------------------------|-------------------|
| `networking.k8s.io/v1` | `ingressclasses` | `list`, `get`, `watch` | Cluster-wide      |
| `networking.k8s.io/v1` | `ingresses`      | `list`, `get`, `watch` | Namespace-scoped* |

> [!NOTE]
> **Namespace Scope:**
> The tool supports the `--namespaces` flag.
> If specific namespaces are provided, permissions are only required for those namespaces.
> If no namespaces are specified, the tool will attempt to analyze all namespaces, and requiring permission across all namespaces for Ingresses.

### Why These Permissions?

The tool uses Kubernetes client-go informers to efficiently cache and monitor Ingress and IngressClass resources.

Informers require `list`, `get`, and `watch` permissions to:
- **list**: Retrieve all existing resources for initial analysis
- **get**: Fetch individual resources when needed
- **watch**: Receive updates when resources change (for report refresh functionality)

All operations are read-only - the tool never modifies any cluster resources.

## Send Report Feature

The Ingress NGINX Migration tool includes an optional feature to share anonymized usage statistics with Traefik Labs.
This helps the Traefik team understand real-world NGINX Ingress usage patterns and prioritize compatibility improvements for the migration process.
The data is transmitted securely over HTTPS to Traefik Labs.

### Privacy and Data Protection

**Important: No sensitive data is transmitted.**
The tool only sends aggregated statistics and counts - never actual ingress configurations, resource names, namespaces, or any other identifying information from your cluster.

### Data Transmitted

When you choose to share your report, only the following anonymized data is sent:

```json
{
  "generationDate": "2024-12-03T14:30:25.123Z",
  "version": "v1.0.0",
  "ingressCount": 42,
  "compatibleIngressCount": 38,
  "vanillaIngressCount": 15,
  "supportedIngressCount": 23,
  "unsupportedIngressCount": 4,
  "unsupportedIngressAnnotations": {
    "nginx.ingress.kubernetes.io/custom-annotation": 2,
    "nginx.ingress.kubernetes.io/experimental-feature": 2
  }
}
```

### How to Use

**Via Web Interface:**
1. Open the migration report in your browser
2. Click the "Share Report" button in the report interface
3. Optionally view the exact data to be sent before confirming

## Utility endpoints exposed by the Ingress NGINX Migration tool

| Method | Path      | Description                     |
|--------|-----------|---------------------------------|
| `GET`  | `/`       | Serve the HTML migration report |
| `PUT`  | `/send`   | Send usage data to Traefik Labs |
| `PUT`  | `/update` | Update the migration report     |
