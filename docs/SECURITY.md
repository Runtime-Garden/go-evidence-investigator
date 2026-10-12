# Security model and limitations

**Threat model:** Public web user, stolen credentials, untrusted PDF/abstract content (prompt injection), malformed model JSON, hallucinated citations, provider outages, unauthorized review, persistent record tampering, and runaway request cost.

## Implemented boundaries

1. Public server startup requires two different 24+ char credentials: `APP_AUTH_TOKEN` and `APP_REVIEW_TOKEN`, plus `APP_REVIEWER_ID`. API endpoints require bearer authentication; reviews additionally require a separate `X-Review-Token` header. Frontend does not persist secrets in browser storage.
2. App config validates provider URLs: HTTPS, except loopback HTTP for local mocks. Authorization tokens are environment-only; logs never print claims, response bodies or secrets.
3. Inputs cap claim length (1,000 chars), body size (32 KiB), evidence count (10) and output sizes (512 KiB LLM / 256 KiB Jev). HTTP POST investigations are bounded at 8 concurrent requests. LLM and Jev calls have context deadlines and limited retries. These limits do not replace a reverse proxy rate limiter.
4. Retrieved documents are treated as **untrusted text**, not agent instructions. Analyst annotations require original source IDs and exact quoted substrings. Narrator must reproduce the final policy verdict and only quote from supplied passages. Exact match does **not** establish semantic correctness.
5. Case records use per-file 0600 permissions, atomic replacement, hash-chain integrity verification and optimistic review versions. The integrity chain is not a cryptographic guarantee against a host administrator able to rewrite every file.
6. Docker: `scratch` runtime, non-root UID, read-only root filesystem, no capabilities, bounded CPU/RAM, persistent named volume, separate proxy network, no host application ports.

## Deployment controls still required

- Put authenticated web UI behind NPM TLS; Cloudflare Access can provide a second identity check. Configure origin bypass protections so the origin cannot be reached directly by an unauthorized visitor.
- Keep `.env` outside Git and backup encrypted. Rotate both keys if compromised. Do not place them in issue reports/screenshots.
- Protect `case_state` volume (contains user investigation text and reviews); define retention limits and offsite backups. `docker compose down -v` **deletes case data**: do not use it casually.
- Review provider privacy/data retention and privacy law obligations before processing personal or proprietary claims. Public SciFact demo is the default.
- Enable NPM/Cloudflare rate limits, access logs with data minimization, OS security updates and secret-scanning for public release.
- Add account-specific RBAC/2FA and proper audit signing before using this outside a small, single-reviewer lab.

## Residual risks

- LLM classifications may be semantically wrong even with verbatim quotes; Jev may be confidently wrong. The review gate is always required.
- In-process metrics are lost on restart; persistent traces include review event metadata but not every HTTP request or provider raw response. Use structured external log collection as needed, without disclosing claim contents.
- Bearer tokens in a page are exposed to scripts/extensions already trusted by the browser. CSP and safe DOM text operations reduce but do not eliminate this risk.
- Single-node JSON files do not provide distributed transactions. Scale beyond a single process by migrating to a transactional database and identity system.
