# For stricter image pinning, replace these version tags with reviewed digests.
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
# No third-party Go modules: the build does not download packages.
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux GOPROXY=off GOSUMDB=off go build -trimpath -ldflags='-s -w' -o /out/evidence-investigator ./cmd/evidence-investigator

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/evidence-investigator /app
COPY --chown=65532:65532 data/demo/ /data/demo/
# Seed named volume with a non-root-owned directory/file. Docker initializes
# a new named volume from this directory on its first mount.
COPY --chown=65532:65532 deploy/state.keep /state/.keep
USER 65532:65532
ENV LISTEN_ADDR=:8080 DATA_DIR=/data/demo CASE_DIR=/state
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/app", "-mode=healthcheck"]
ENTRYPOINT ["/app"]
