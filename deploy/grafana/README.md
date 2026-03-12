# SandboxMatrix Grafana Dashboards

Pre-built Grafana dashboard templates for monitoring SandboxMatrix metrics exposed via Prometheus.

## Dashboards

| Dashboard | File | Description |
|-----------|------|-------------|
| **Overview** | `dashboards/overview.json` | Active resources, operation rates, HTTP traffic, activity timeline |
| **Performance** | `dashboards/performance.json` | HTTP latency percentiles, heatmap, operation durations, throughput, error rate |
| **Resources** | `dashboards/resources.json` | Capacity gauges, utilization trends, operations breakdown, connection monitoring |

## Prerequisites

- Grafana 10.0 or later
- Prometheus datasource configured and scraping the SandboxMatrix `/metrics` endpoint

## Quick Setup with Provisioning

1. Copy the dashboard JSON files to your Grafana dashboards directory:

```bash
cp -r dashboards/ /var/lib/grafana/dashboards/sandboxmatrix/
```

2. Copy the provisioning configuration:

```bash
cp provisioning/dashboards.yaml /etc/grafana/provisioning/dashboards/sandboxmatrix.yaml
```

3. Restart Grafana. The dashboards will appear in the **SandboxMatrix** folder.

## Docker Compose Example

Add the following services to your `docker-compose.yml`:

```yaml
services:
  sandboxmatrix:
    image: sandboxmatrix:latest
    ports:
      - "8080:8080"

  prometheus:
    image: prom/prometheus:v2.51.0
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml

  grafana:
    image: grafana/grafana:10.4.0
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - ./deploy/grafana/dashboards:/var/lib/grafana/dashboards/sandboxmatrix
      - ./deploy/grafana/provisioning/dashboards.yaml:/etc/grafana/provisioning/dashboards/sandboxmatrix.yaml
```

## Prometheus Scrape Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'sandboxmatrix'
    scrape_interval: 10s
    static_configs:
      - targets: ['sandboxmatrix:8080']
    metrics_path: /metrics
```

## Manual Import

If you prefer not to use provisioning, you can import each dashboard manually:

1. Open Grafana in your browser (default: `http://localhost:3000`)
2. Navigate to **Dashboards > Import**
3. Click **Upload dashboard JSON file** and select a JSON file from the `dashboards/` directory
4. Select your Prometheus datasource when prompted
5. Click **Import**

Repeat for each dashboard file.

## Metrics Reference

The dashboards query the following Prometheus metrics exposed by SandboxMatrix:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `smx_sandboxes_active` | Gauge | - | Number of active (running/ready) sandboxes |
| `smx_sessions_active` | Gauge | - | Number of active sessions |
| `smx_matrices_active` | Gauge | - | Number of active matrices |
| `smx_websocket_connections` | Gauge | - | Number of active WebSocket connections |
| `smx_pool_size` | Gauge | `blueprint` | Number of pre-warmed sandbox instances per blueprint |
| `smx_sandbox_operations_total` | Counter | `operation`, `result` | Total number of sandbox operations |
| `smx_sandbox_operation_duration_seconds` | Histogram | `operation` | Duration of sandbox operations |
| `smx_exec_total` | Counter | `sandbox`, `result` | Total number of exec commands |
| `smx_exec_duration_seconds` | Histogram | `sandbox` | Duration of exec commands |
| `smx_http_requests_total` | Counter | `method`, `path`, `status` | Total number of HTTP API requests |
| `smx_http_request_duration_seconds` | Histogram | `method`, `path` | Duration of HTTP API requests |

## Template Variables

Each dashboard includes the following template variables:

- **DS_PROMETHEUS** -- Datasource selector for Prometheus (allows switching between Prometheus instances)
- **interval** -- Rate interval with auto option (1m, 5m, 15m, 1h)
