# Grafana Dashboards

Files: [`deployments/grafana/`](../../deployments/grafana/) —
`dashboards/*.json` (the dashboards themselves) and
`provisioning/` (Grafana's own config-as-code for loading them and a
datasource automatically). Visualizes
[`internal/metrics`](../../internal/metrics)'s Prometheus output (Day
17).

## Problem

Day 17 made every capability's aggregate signal queryable via
Prometheus, but only as raw PromQL — useful for someone who already
knows what to ask for, not for a glance-able operational view.
Grafana, named in this project's own design principles and the last
stage of the architecture diagram in `README.md`, is the standard tool
for that. This day's job is narrow: define real, checked-in dashboard
JSON against metrics that actually exist, and verify — genuinely,
against a real Grafana server, not just "the JSON parses" — that
Grafana itself accepts them.

## Design

```
deployments/grafana/
├── dashboards/
│   └── pulse-overview.json      # 6 panels, one per internal/metrics family
└── provisioning/
    ├── datasources/datasource.yml   # Prometheus datasource, config-as-code
    └── dashboards/provider.yml      # loads dashboards/*.json automatically
```

**Dashboard-as-code, not a UI artifact.** `pulse-overview.json` is a
plain Grafana dashboard JSON file, hand-written the same way every
other piece of config in this project is: readable, reviewable in a
diff, and — unlike clicking through Grafana's UI and exporting — never
carrying UI session state (Grafana's `id` field, `iteration`
timestamps) that would make every export a spurious diff. It's built
directly from `internal/metrics`'s own metric names: every panel's
PromQL expression queries a metric family `docs/design/metrics.md`
actually documents — `pulse_events_total`, `pulse_network_connect_total`,
`pulse_network_bytes_sent_total`/`_received_total`,
`pulse_http_requests_total`, `pulse_dns_query_latency_ms`,
`pulse_dns_response_code_total` — nothing invented for the dashboard
that isn't already real.

**A placeholder datasource UID, substituted at import time, not a
hardcoded one.** Every panel's `datasource.uid` is the literal string
`PULSE_PROMETHEUS_DATASOURCE_UID` — a real Prometheus datasource's UID
is only known once one actually exists in a given Grafana instance
(Grafana assigns it), so the checked-in file can't hardcode a real
one. The provisioning path (`datasources/datasource.yml`) sidesteps
this by naming the datasource, which Grafana's dashboard provisioner
resolves by name at load time; CI's own verification path (see below)
resolves the same placeholder by substitution instead, since it drives
Grafana's HTTP API directly rather than the provisioning-file path.

**Verified against a real Grafana server, not just parsed as JSON.**
CI's `grafana-integration` job runs a real `grafana-oss` service
container, creates a real Prometheus datasource via Grafana's own API,
substitutes its real UID into the dashboard JSON, and imports it via
`POST /api/dashboards/db` — the same endpoint the Grafana UI itself
uses to save a dashboard. A `200` response means Grafana's own schema
validation accepted the panel/target structure; this is real
verification of dashboard *validity*, not a rendering or data check
(see Limitations).

**Provisioning files document the real deployment path; CI verifies a
different, more testable path to the same acceptance question.**
`datasources/datasource.yml` and `dashboards/provider.yml` are what an
operator actually mounts into a Grafana container's
`/etc/grafana/provisioning/` tree — the standard Grafana config-as-code
mechanism, requiring no manual UI setup. CI doesn't test that specific
mounting mechanism (it would need a custom-built Grafana image rather
than the stock service container this project's other integration
jobs already use); it tests the dashboard JSON's acceptance by
Grafana's API directly instead, which is a strictly stronger check on
the property that actually matters — is this dashboard well-formed and
importable — even though it doesn't exercise the provisioning YAML
files themselves.

## Tradeoffs

- **One dashboard, not several.** `internal/metrics` has one coherent
  metric set (see `docs/design/metrics.md`); splitting it across
  multiple dashboards (per-capability, say) would be organizational
  overhead without a second real metric source yet to justify it.
- **No ClickHouse-backed panels.** Grafana's ClickHouse support needs
  a separate plugin (`grafana-clickhouse-datasource`) not bundled in
  the stock `grafana-oss` image this project's CI already uses for
  Prometheus-only verification — adding it would mean either a custom
  image build or an unverified plugin-install step. `pulse-cli
  topology` (Day 16) already covers the ClickHouse-backed dependency
  view; this day stays Prometheus-only rather than adding a second,
  less-verifiable integration for a view that already exists elsewhere.
- **The datasource's `url` in CI points at `http://localhost:9090`,
  which nothing in that job serves.** Fine for what's being verified
  (dashboard JSON acceptance): Grafana accepts a datasource
  configuration without ever querying it, the same way saving one in
  the UI doesn't require "Save & Test" to succeed first. It does mean
  CI never confirms a panel's PromQL actually returns sensible data —
  see Limitations.

## Failure modes

An import that fails (non-`200` from `/api/dashboards/db`) fails the
CI job with the response body surfaced via the same `::error::`
annotation pattern `kafka-integration` and `clickhouse-integration`
already use, for the same reason: no authenticated GitHub access to
read raw job logs.

## Performance

Not applicable — dashboards are static configuration, not code with a
runtime cost of their own beyond whatever a real Grafana server (not
part of this project) spends rendering them.

## Security

The CI job sets an explicit, CI-only admin password
(`GF_SECURITY_ADMIN_PASSWORD`) rather than relying on `grafana-oss`'s
own default, so a basic-auth API call has a password it can rely on
regardless of image defaults — this is a throwaway, ephemeral
container credential, not a real Grafana instance's admin password,
and never persists past the job.

## Limitations

- **No live-data verification.** CI confirms Grafana accepts each
  dashboard's structure; it does not confirm a panel's PromQL query
  returns sensible results against real Prometheus data, because no
  such data exists in that job — there is no Prometheus server there,
  and no telemetry flowing through one. That's a real gap: a
  syntactically valid PromQL expression that queries the wrong thing
  would pass this verification.
- **No ClickHouse-backed panels** — see Tradeoffs.
- **Provisioning YAML files aren't exercised by CI** — see Design's
  note on what's actually verified vs. what's just documented as the
  real deployment path.
