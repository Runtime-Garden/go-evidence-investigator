# Runtime Garden deployment & operations

**Target:** Ubuntu Server 24.04 LTS, `/opt/stacks/evidence-investigator`, Docker Engine/Compose, Dockge, NGINX Proxy Manager and external Docker network `proxy`.

These are instructions, **not claims of a completed deployment**. Docker and VPS commands below must be executed by an authorized operator. Do not assume the host has spare RAM/disk or a working origin firewall.

## 1. Prerequisites

- Verify `docker compose version`, `docker network inspect proxy`, free disk/RAM and that NPM is joined to `proxy`.
- Transfer the whole source tree to `/opt/stacks/evidence-investigator` (Git checkout or protected SCP). Never upload live `.env` to public repos.
- Copy `.env.example` to `.env`; set two distinct 64-hex secrets, reviewer ID, LLM and JEV credentials, `chmod 600 .env`.
- Create the stack only in Dockge/Compose; it references existing `proxy` external network. No public TCP host port is required for the app.

## 2. Build and start

```sh
cd /opt/stacks/evidence-investigator
cp .env.example .env       # edit secrets first
chmod 600 .env
docker network inspect proxy >/dev/null
docker compose config --quiet
docker compose build
docker compose up -d
docker compose ps
docker compose logs --tail 40 evidence-investigator
```

**Ensure `APP_AUTH_TOKEN`, `APP_REVIEW_TOKEN`, and `APP_REVIEWER_ID` have real values before starting.** Do not paste `.env` contents or full `docker inspect` output into tickets.

NPM Proxy Host should target `http://evidence-investigator-runtimegarden:8080` on the shared proxy network, with a deliberately selected subdomain, TLS and access controls. The proposed DNS name is not automatically provisioned. Do not expose raw port `8080` publicly. For restricted lab access, add Cloudflare Access AND verify origin bypass protection.

## 3. Smoke checks

```sh
# Container liveness only (not downstream providers):
docker compose exec -T evidence-investigator /app -mode=healthcheck
# From a host capable of reaching the published proxy:
./deploy/verify.sh https://YOUR-LAB-DOMAIN.example
# Additional end-to-end provider check must be run manually with test inputs.
```

What `verify.sh` checks: public liveness, missing token is rejected (401), authenticated `/api/info`, and returns basic health + provider configuration readiness. It intentionally avoids paid inference calls. Real Jev / LLM correctness requires user-supplied credentials and non-synthetic evaluation.

## 4. Backup and retention

`case_state` contains claims and human review history. Do not wipe it. Example non-destructive local snapshot:

```sh
cd /opt/stacks/evidence-investigator
./deploy/backup.sh
# Copy the generated private backup to encrypted offsite storage.
```

Review-event files must be kept consistent during backup; the script uses `docker cp` on live data, which is fine for small immutable snapshots but can race with concurrent writes. For guaranteed consistent backups, temporarily stop new writes or take a filesystem snapshot and then validate each case; retain a separate encrypted offsite copy. Schedule retention/deletion policy only after stakeholder review; no automatic case deletion is configured.

## 5. Upgrade and rollback

Record current Git commit and image digest; keep known-good version available and back up state first. Stage new version, run local test commands, deploy via `docker compose up -d --build`, then health and auth verification. If something fails, restore the previous commit and rebuild/recreate only the app container, without modifying its named volume. Review app schema migrations before rollback; current JSON format has no DB migrations.

**Dangerous:** `docker compose down -v`, `docker volume rm`, arbitrary recursive `rm`, and `docker system prune --volumes` can destroy review history. Do not use those commands for rollback.

## 6. Observability

- Structured HTTP/ startup logs contain route labels and error metadata (do not enable body logging).
- `GET /api/metrics` (authenticated) returns in-process per-stage request count, errors and elapsed time totals. The metrics reset on process restart.
- Check errors, latency and actual token usage with benchmark output. Alerts, persistent tracing and cost dashboards are future operational enhancements, not already deployed services.
- `GET /healthz` is a liveness check only: a healthy process does **not** prove JEV/LLM provider connectivity or scientific correctness.

## 7. Restore rehearsal

1. Install fresh Compose stack with an **empty** volume name and restricted network access.
2. Restore an encrypted test copy of the backed-up `/state` directory while the app is stopped; preserve file modes/UID.
3. Restart and use authenticated `GET /api/cases/{id}` to verify review hash chains and expected versions.
4. Confirm auth, health and provider configuration, then separately test a safe investigation and a review on a disposable case.
5. Only promote restored state after a full rehearsal. Do not overwrite the active volume without explicit approval.
