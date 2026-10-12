#!/bin/sh
set -eu
base=${1:?usage: verify.sh https://YOUR-APP-DOMAIN}
: "${APP_AUTH_TOKEN:?export APP_AUTH_TOKEN for authenticated verification}"
base=${base%/}
case "$base" in https://*) ;; *) echo 'Refusing non-HTTPS URL' >&2; exit 2;; esac
printf '%s\n' '1. Healthcheck'
curl -fsS --max-time 10 "$base/healthz"
printf '\n%s\n' '2. Unauthorized request should return 401'
status=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$base/api/info")
[ "$status" = 401 ] || { echo "Expected 401, got $status" >&2; exit 1; }
printf '%s\n' '3. Authenticated dataset/provider information'
curl -fsS --max-time 10 -H "Authorization: Bearer $APP_AUTH_TOKEN" "$base/api/info"
printf '\n%s\n' 'PASS: smoke checks (NOT a live provider evaluation)'
