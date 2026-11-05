# TLS Follow-up — REST and gRPC

- **Status:** Deferred from US09 (#19), pending a dedicated issue
- **Date:** 2026-11-04
- **Applies to:** Orchestrator REST API, Sentinel REST client, propagation gRPC
  channel

## 1. Scope

US09 ships the propagation channel in plaintext, matching the existing REST
API. This follow-up adds TLS to **both** channels:

- Orchestrator REST API (`cmd/orchestrator`, `internal/orchestrator/api`).
- Sentinel REST client (`internal/sentinel/client`).
- Propagation gRPC server and client (`internal/orchestrator/api/grpcserver`,
  `internal/sentinel/client`).
- Certificate provisioning, trust configuration, and the e2e fixtures needed to
  exercise both channels.

TLS on the gRPC channel alone is not enough: the session token is issued in the
REST authentication response, so a plaintext REST channel exposes the
credential that authorizes the gRPC stream. Protecting the rules without
protecting the token leaves a replay path.

## 2. Goals

1. Rules, ACKs, and the session token are encrypted and integrity-protected in
   transit on both channels.
2. The Sentinel verifies the Orchestrator's certificate; no "skip
   verification" mode in production.
3. Existing machine authentication (HMAC-signed session token) remains the
   machine identity. TLS is the transport, not a replacement for the token.
4. The e2e suite proves a TLS handshake on both channels.

## 3. Non-goals

- mTLS / per-Sentinel client certificates. It is a valid alternative to the
  session token, but it adds certificate provisioning for every machine and is
  a separate decision.
- Certificate rotation and automated renewal.
- TLS termination at a reverse proxy. It can be added later; this follow-up
  terminates TLS in the Orchestrator.
- Changes to the propagation message, sequence, epoch, or audit model.

## 4. What Changes

### 4.1 Orchestrator

- Add TLS configuration: certificate path, key path, and whether TLS is
  required.
- REST: serve with `http.Server` + `TLSConfig` (or `ListenAndServeTLS`).
- gRPC: pass `grpc.Creds(credentials.NewServerTLSFromFile(cert, key))` to
  `grpc.NewServer`.
- Fail fast when TLS is required and the certificate or key cannot be loaded.

### 4.2 Sentinel

- Add a CA file (or the server certificate) used to verify the Orchestrator.
- REST: give `http.Client` a `Transport` with `TLSClientConfig.RootCAs`.
- gRPC: replace `insecure.NewCredentials()` with
  `credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: ...})`.
- Support `https://` in `orchestrator_url`; keep the scheme validation in
  `internal/sentinel/config`.

### 4.3 Configuration

Additive fields, not replacements:

- Orchestrator: certificate/key paths and a TLS-required switch.
- Sentinel: CA path, plus the existing `orchestrator_url` and
  `propagation_address`.
- `.env.example` and `examples/sentinel.config.json` document the new fields.

### 4.4 E2E and Tests

- Generate a test CA and a server certificate with SANs for the names the
  clients use.
- Mount the certificates into the Orchestrator and Sentinel containers.
- Configure both sides to use TLS and assert the propagation flow still works.
- Keep `bufconn` unit tests on insecure transport for speed; add one TLS
  handshake test per channel.
- Add a negative test: an untrusted certificate is rejected.

## 5. What Stays the Same

- `proto/propagation.proto` and `internal/common/propagation`.
- Domain types, services, stores, sequence, epoch, and audit rows.
- The session token format and `auth.SignSessionToken` /
  `auth.ParseSessionToken`.
- The propagation state file format.
- The HTTP routes and gRPC method signature.

## 6. Seams to Preserve Now

These keep the follow-up a small diff:

1. Keep gRPC server and client construction in one place, not scattered across
   handlers or services.
2. Keep `orchestrator_url` and `propagation_address` explicit and independent;
   do not derive one from the other in a way that assumes `http`.
3. Keep the dial target a real `host:port` that can also serve as the TLS
   server name. Avoid an IP-only assumption in the Sentinel config.
4. Keep the session token as the machine identity and TLS as the pipe; do not
   make one depend on the other.
5. Prefer constructors that can accept transport options later (a
   `grpc.ServerOption` / `grpc.DialOption` seam) without implementing TLS now.

## 7. Hostname and SAN Decision

The Sentinel must dial a name or IP covered by the certificate:

- In Docker/e2e, `orchestrator` is a stable DNS name on the fleet network.
- The current e2e harness uses static IPs (`172.30.0.10`). A certificate for
  that address needs an IP SAN, or the harness needs a DNS alias.
- The chosen `propagation_address` and `orchestrator_url` must match the
  certificate SAN, or the client must set `ServerName` explicitly.

This decision must be made before the certificate fixtures are written.

## 8. What Deferring Does Not Save

Certificate provisioning, the SAN/hostname decision, and rotation cost the
same whenever they are done. Deferring only postpones them. Until this
follow-up lands, rules and the bearer token travel in plaintext; that is
acceptable only on a trusted lab network.

## 9. Done Criteria

1. REST and gRPC both serve TLS; the Sentinel trusts the configured CA.
2. The Sentinel rejects an untrusted or mismatched certificate.
3. A full propagation cycle (connect, package, ACK, change, revoke) works over
   TLS in e2e.
4. The session token is never sent over a plaintext channel.
5. `make tests`, `make coverage`, and `make e2e` pass with the new fixtures.
