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
   --help, -h           show help
```

## Installation

```bash
go build -o ingress-nginx-analyzer ./cmd
```

## Running

### Local
Run locally with a kubeconfig file:

```bash
./ingress-nginx-analyzer --kubeconfig ~/.kube/config
```

### In-cluster
Deploy the analyzer as a pod in your Kubernetes cluster where it can automatically discover and analyze Ingress resources.

```bash
kubectl apply -f - <<EOF
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ingress-nginx-analyzer
  namespace: default

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ingress-nginx-analyzer
rules:
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses"]
  verbs: ["get", "list", "watch"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ingress-nginx-analyzer
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ingress-nginx-analyzer
subjects:
- kind: ServiceAccount
  name: ingress-nginx-analyzer
  namespace: default

---
apiVersion: v1
kind: Pod
metadata:
  name: ingress-nginx-analyzer
  namespace: default
  labels:
    app: ingress-nginx-analyzer
spec:
  serviceAccountName: ingress-nginx-analyzer
  securityContext:
    runAsNonRoot: true
    runAsUser: 65534
    fsGroup: 65534
  containers:
  - name: ingress-nginx-analyzer
    image: traefik/ingress-nginx-analyzer:latest
    ports:
    - containerPort: 8080
      name: http
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
        - ALL
EOF
```

Access the report:
```bash
kubectl port-forward pod/ingress-nginx-analyzer 8080:8080
# Open http://localhost:8080 in your browser
```


