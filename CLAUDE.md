# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Project Overview

Go web application that analyzes Kubernetes NGINX Ingress resources for migration to Traefik. The HTML template at [pkg/handlers/report.html](pkg/handlers/report.html) is the **primary file for design work**.

Key files:

- `pkg/handlers/report.html` - Migration report template (main work area)
- `cmd/main.go` - HTTP server with two endpoints: `/` (report), `/send-report` (submit)
- `pkg/analyzer/report.go` - Template data structure

## Development

```bash
go run ./cmd --addr=:9090 --kubeconfig ~/.kube/config
open http://localhost:9090
```

## Template Data

Template variables (see `pkg/analyzer/report.go`):

- `.IngressCount`, `.CompatibleIngressCount`, `.UnsupportedIngressCount`
- `.CompatiblePercentage`, `.UnsupportedPercentage`
- `.UnsupportedIngressAnnotations` - Map of annotation → count
- `.UnsupportedIngresses` - Array with `.Name`, `.Namespace`, `.IngressClassName`, `.UnsupportedAnnotations`
- `.ReportJSON`

## Design System

**CSS:**

- Plain CSS only, no preprocessors
- CSS variables in `:root`, styles in `<style>` tag in `<head>`
- NO CSS comments, transitions, or animations unless specified
- Use class selectors (`.button`, `.button-large`), NOT element selectors
- Size variants only override changed properties, don't repeat defaults
- Theme: `[data-theme="dark"]` for dark mode, managed via JS/localStorage
- Keep DESIGN.md in sync with changes

See [DESIGN.md](DESIGN.md) for specs.

## Implementation Guidelines

**Tables:**

- Apply box-shadow to `.table-container`, NOT `.table` element (prevents clipping from overflow-x)
- Use elevation 2 ($02dp background + elevation 2 shadow) when table is inside elevation 1 card
- Use `border-collapse: separate` with `border-spacing: 0` to allow rounded corners
- Border pattern: apply `border-bottom` to cells, remove on last row

**Cards with tables:**

- Use `.card-no-padding` class to remove card padding when it contains tables
- Apply padding to `.section` wrapper inside the card instead
- Keep DOM flat inside `.section`: h2, p, and table-container as direct children
- No intermediate wrapper divs like `.section-header` or `.section-content`

**Example structure:**

```html
<div class="card card-elevation-1 card-no-padding">
  <div class="section">
    <h2>Title</h2>
    <p>Description</p>
    <div class="table-container">
      <table class="table">...</table>
    </div>
  </div>
</div>
```
