# GinBoot Example Application

This is an example application showcasing how to use the `ginboot` framework.

## Telemetry with Grafana Cloud

This example is pre-configured to send Traces, Metrics, and Logs via OpenTelemetry to **Grafana Cloud**!

### 1. Configure Credentials
1. Copy the environment variables template:
   ```bash
   cp .env.example .env
   ```
2. Log into [Grafana Cloud](https://grafana.com/).
3. In your stack, navigate to **Connections -> Add new connection -> OpenTelemetry**.
4. Choose your deployment method and find the **OTLP Endpoint** and **Basic Auth Password (API Token)**.
5. Base64 encode your `instance_id:api_token`:
   ```bash
   echo -n "YOUR_INSTANCE_ID:YOUR_API_TOKEN" | base64
   ```
6. Paste the resulting string into `.env` under `OTEL_EXPORTER_OTLP_HEADERS`.
7. Update `OTEL_EXPORTER_OTLP_ENDPOINT` with the URL provided by Grafana.

### 2. Run the Application
Source the `.env` file and run the application:

```bash
# Load the environment variables
source .env

# Run the app
go run cmd/main.go
```

### 3. View Data in Grafana
1. Send some HTTP requests to the app (e.g. `GET http://localhost:8080/api/v1/posts`).
2. Open your Grafana Cloud dashboard.
3. Go to **Explore**.
4. You can now query your metrics (Prometheus), logs (Loki), and traces (Tempo) for the `ginboot-example` service!
