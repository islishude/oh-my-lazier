# Kubernetes worker monitoring

Operational guidance for Kubernetes worker deployments.
Retain the existing approved config, signer integration, and network policy.
Select a tested immutable image reference for each release and follow
[configuration review](runbooks/config-diff.md).

## Probe and scrape configuration

- A suitable ServiceMonitor interval is 30 seconds with a 10-second
  `scrapeTimeout`, leaving room for the exporter's three-second database budget.
- A startup probe can check `/healthz` on port `metrics` every five seconds,
  with a three-second timeout and 120 failures allowed (approximately ten
  minutes). Readiness and liveness probes wait for startup success.
- Use an exec readiness probe invoking `readinesscheck` with the worker config. Business failures
  can make the Pod NotReady; they do not disable `/metrics`.
- ServiceMonitor discovers endpoint Pod addresses, rather than scraping via
  ClusterIP. Keep NotReady endpoints discoverable. Do not add readiness-based
  relabel filters or rewrite the scrape address to the Service IP. No
  `publishNotReadyAddresses` override is needed for this discovery pattern.
- Metrics starts after initialization; startup failures may have no listening
  exporter. Rollout availability depends on replica count and update strategy;
  a single-replica `Recreate` deployment can have scrape gaps.

Update the existing ServiceMonitor in its own namespace rather than creating
a duplicate. Confirm the Prometheus object's namespace and label
selectors select both the ServiceMonitor and PrometheusRule. Add the release
labels required by that installation to their metadata.

Load the common [worker alert rules](monitoring/prometheus-alerts.yml)
through the existing rule-management process. They now use the directly scraped
`laz_worker_readiness` gauge and do not need a blackbox readiness probe.
The [scrape rule template](monitoring/kubernetes-alerts.yml) uses explicit
`replace-me-namespace` and `replace-me-service` placeholders. Before loading it,
replace these values and the `endpoint` selector with the target labels from
your Prometheus installation. It includes:

- `LazWorkerScrapeFailed`: `up{namespace="replace-me-namespace",service="replace-me-service",endpoint="metrics"} == 0`
  for two minutes, severity page.
- `LazWorkerTargetMissing`: `absent(up{namespace="replace-me-namespace",service="replace-me-service",endpoint="metrics"})`
  for five minutes, severity page. This catches discovery removal, which cannot
  be detected by `up == 0` alone. Install only where this deployment is expected.

Evaluate these deployment rules in the cluster-local Prometheus. For a central
multi-cluster rules engine, create an expected-target rule per cluster with its
cluster label selector; another cluster's target must not hide a missing one.
Retire the missing-target rule when intentionally retiring the deployment.

## Read-only production inspection

Use the approved cluster context and inspect the existing resources, without
printing ConfigMaps, Secret contents, or complete credential-bearing scrape
configuration:

Inspect the worker Pods and the Service's EndpointSlices in their configured
namespace. Use the actual workload selector and Service name.

In Prometheus Targets/Service Discovery, locate the job for this ServiceMonitor.
Confirm that its final address is the Pod IP and configured metrics port, its timeout matches the ServiceMonitor,
and it has the expected `namespace`, `service`, and `endpoint` labels. Inspect
only this job's relabel rules from the generated configuration: none may drop
`__meta_kubernetes_pod_ready`, `__meta_kubernetes_endpoint_ready`, or
`__meta_kubernetes_endpointslice_endpoint_conditions_ready=false`. Global scrape
classes can add filters not visible in the ServiceMonitor. Remove any such filter
only for this worker monitoring job. Confirm both worker and scrape alerts
appear in Prometheus Rules and route to the existing page receiver.

## Isolated acceptance checklist

Do not inject failures into production or live cross-chain execution. Use a
separate test cluster and disposable database with synthetic worker state and
local chain services. Record the image, Operator version, generated job,
probe results, target labels, and rule results for each scenario.

1. Healthy baseline: `readinesscheck` exits 0; the Pod is Ready; the direct Pod
   target has `up=1` and `laz_worker_readiness=1`.
2. Introduce a paused chain or manual-review job in the disposable database.
   After readiness thresholds, confirm exit 2 and Pod NotReady. The same target
   must remain discovered with `up=1`, `laz_worker_readiness=0`, and business
   metrics updating. After two minutes the readiness alert must fire.
3. Make database reads fail, then separately block statistics queries/pool
   acquisition. `/metrics` must return within the 10-second scrape budget with
   HTTP 200, runtime metrics, and `laz_metrics_db_snapshot_available=0`; business
   metrics including readiness must be absent. Verify the two-minute database
   alert and recovery after restoring the database.
4. Make queued preflight return nil, zero, and negative priority suggestions.
   Verify repeated Warn logs, unchanged nonce/attempt budgets, a page at five
   minutes, and automatic recovery. This fault alone must not fail readiness.
5. Keep the discovered test target but stop its listener or block only its scrape
   traffic: verify `up=0` and the two-minute scrape alert. Separately remove the
   target from discovery: verify the five-minute missing-target alert, even if
   an unrelated namespace still has a healthy worker. Restore and verify both
   alerts resolve.
6. Delay test initialization within the startup budget: confirm liveness does
   not restart the container before startup succeeds. A single-replica Recreate
   update may have a target gap; prolonged absence must still alert.

## Upgrade and evidence

Deploy the incremental `005_tx_preflight.sql` migration through normal worker
startup and publish the matching worker metrics and alert rules together. No
old migrations are changed and no history is backfilled. Existing blocked
queued rows begin a new observed episode on their next failed check.

`make check-alerts` validates both the common rules and deployment-specific
scrape rules under `docs/monitoring`. Load the scrape rule groups through the
existing rule-management process (for Prometheus Operator, place them under
a PrometheusRule `spec`). Local handler tests do not prove cluster target discovery.

## Validation record

Validation date: 2026-10-13. Results below describe local checks, not production
or Kubernetes acceptance evidence.

### Local evidence

- `make check`: passed compilation, type checking, ABI drift checks, contract and
  script tests, Go tests, runbook validation, lint, formatting, and alert tests.
- Focused Postgres tests: passed observation ordering, restart-visible history,
  recovery, scope filtering, nonce-claim fencing, database query cancellation,
  and exhausted connection-pool cancellation.
- Metrics handler tests: passed the three-second snapshot timeout, request
  cancellation, recovery, role-aware readiness, and business failures retaining
  HTTP 200 metrics independently of HTTP 503 readiness.
- Promtool tests: passed preflight 299/300-second boundaries, recovery, scrape
  failure, target disappearance, and namespace/instance isolation.
- `make docker-smoke`: passed image build and worker entrypoint check.
- `make security-check`: passed the repository's security review and npm audit
  disposition gates; govulncheck reported no affected Go symbols. This does not
  mean the npm dependency tree has no audit findings.
- The scrape rule template and its synthetic target fixtures are validated by
  `make check-alerts`.
- `make test-integration INTEGRATION_MANAGE_DEPS=0`: passed with the local
  Compose Postgres and Rustack KMS dependencies, including the full serial Go
  suite and the recovery race gate. The race selector now includes preflight
  observation and recovery tests.
- Final focused database and transaction-manager checks passed after adding
  upgrade coverage from both migration 003 and migration 004, and checking
  shutdown cancellation, persistence errors, fee-quote errors mentioning revert,
  and repeated priority-fee failures followed by recovery.
- Existing migration files are unchanged; only migration 005 is added.

### Required cluster acceptance

Not executed: no isolated Kubernetes test cluster or deployed Prometheus
configuration was supplied for this change. No production resources, chain
state, or remote scrape configuration were modified or certified.

Before rollout, complete the [isolated acceptance checklist](#isolated-acceptance-checklist),
including NotReady target retention, database timeout with `up=1`, and scrape
failure versus missing-target alerts. Inspect the actual generated scrape job
and rule selectors in the destination installation. This guidance alone cannot
prove that global relabeling or namespace/label selectors permit scraping.
