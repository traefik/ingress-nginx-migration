# Traefik Migration Report

Generated: 2026-05-27T10:00:00Z · tool v0.3.0
Hash: `abc123def456`

## Summary

| Metric | Count | % |
|---|---|---|
| Total | 4 | 100% |
| Compatible | 2 | 50.0% |
| • Vanilla | 1 | 25.0% |
| • Supported | 1 | 25.0% |
| Unsupported | 2 | 50.0% |

## Minimum Traefik version

| v3.6 | v3.7 | Hub |
|---|---|---|
| 1 | 1 | 0 |

## Blocking annotations

| Annotation | Count | Kind |
|---|---|---|
| `nginx.ingress.kubernetes.io/limit-connections` | 2 | unsupported |
| `nginx.ingress.kubernetes.io/totally-made-up` | 1 | unknown |
