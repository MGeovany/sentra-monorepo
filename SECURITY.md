# Security

This document is the formal threat model for Sentra (CLI + local server + Supabase).
It exists to make security decisions explicit and consistent over time.

## Scope

In scope:
- Sentra CLI (OAuth login flow, token handling, local state)
- Sentra server (HTTP API, JWT verification, Supabase DB writes)
- Supabase Auth (OAuth via Supabase, JWKS, token issuance)

Out of scope (for now):
- Supply-chain threats from third-party dependencies (handled separately)
- GitHub OAuth app security settings (owned by Supabase/project configuration)
- Host OS compromise/root-level malware (only partially mitigable)

## Assets (What We Protect)

Primary secrets:
- Access token (Bearer credential)
- Refresh token (longer-lived credential)

Secondary sensitive data:
- User identity metadata (user_id/sub, email)
- Machine identity metadata (machine_id, hostname)
- Local scan results and commit metadata (may include repo paths and env key names)

Non-secrets but integrity-critical:
- CLI local state (e.g., pushed markers)
- Server-side authorization decisions

## Trust Boundaries and Data Flow

1. User runs `sentra login`.
2. CLI starts a loopback HTTP listener on 127.0.0.1 using an ephemeral port.
3. CLI prints Supabase authorize URL (PKCE).
4. Browser authenticates with provider and redirects to CLI callback (loopback).
5. CLI validates callback state nonce and exchanges auth code for tokens with Supabase.
6. CLI stores the session in OS credential store (Keychain/Secret Service/CredMan via go-keyring).
7. CLI calls server endpoints using `Authorization: Bearer <access_token>`.
8. Server verifies JWT using Supabase JWKS and validates claims before accepting requests.
9. Server uses Supabase service role key for controlled DB writes.

Trust boundaries:
- Browser <-> CLI loopback callback (local machine boundary)
- CLI <-> Supabase Auth (network boundary)
- CLI <-> server (local network boundary; may become remote in the future)
- Server <-> Supabase PostgREST (network boundary)

## Attackers (Who We Defend Against)

Local attackers:
- Same-user malware running under the same OS account (high capability)
- Another local OS user without elevated privileges
- Physical access with disk access / backups

Remote attackers:
- Network attacker attempting token interception
- Attacker controlling a website trying to trick a user during OAuth
- API probing / brute force against server endpoints

Insiders / misconfiguration:
- Developer/operator accidentally logging secrets or exposing generic DB passthrough
- Wrong Supabase URL or keys leading to acceptance of tokens from the wrong tenant

## Attack Surface

CLI:
- Loopback callback endpoint (127.0.0.1)
- Printed authorize URL (can be copied/shared)
- Local storage: OS keychain entry + legacy fallback files when explicitly enabled
- Environment variables and .env files

Server:
- Public HTTP endpoints (currently /users/me, /machines/register)
- JWT verification logic (JWKS fetch, alg/kid selection, claim validation)
- Supabase service role key usage (privileged DB access)

Supabase:
- OAuth provider configuration and redirect URLs
- JWKS availability and key rotation

## Security Controls (What We Implement)

### OAuth / Login Hardening (CLI)
- PKCE S256 is used for the authorization code flow.
- Callback listener binds to 127.0.0.1 with an ephemeral port by default.
- A random state nonce is generated and validated on callback.
- Callback is processed only once to reduce races/spoofing.

### Token Storage at Rest (CLI)
- Session is stored in OS credential store (Keychain/Secret Service/CredMan) via `go-keyring`.
- Legacy on-disk session material is removed after successful migration.
- Insecure file fallback is disabled by default; it can only be enabled explicitly via:
  - `SENTRA_ALLOW_INSECURE_SESSION_FILE=true`

### Server Authentication (JWT)
- JWT signature verification via Supabase JWKS.
- Allowed signing methods restricted (RS256/ES256).
- Claims validation is strict:
  - `iss` must match the expected Supabase issuer
  - `aud` must include "authenticated"
  - `exp` is required
  - small leeway to handle clock skew
- Application-level claim hardening:
  - `sub` must be present
  - `role` must be present and must be "authenticated" (allowlist)
- Optional header hardening:
  - if `typ` is present, it must be "JWT"

### Logging
- Avoid logging secrets (tokens, refresh tokens, authorization codes).
- Server logs errors and IDs for observability.

## Failure Modes (What Happens If It Fails)

If tokens leak:
- Attacker can act as the user until token expiration/revocation.
- If refresh token leaks, compromise lasts longer.

If JWT validation is bypassed:
- Attacker can call authenticated endpoints and cause privileged side effects.

If Supabase service role key is misused/exposed:
- Full DB compromise (RLS bypass) is possible.

If loopback callback is spoofed:
- Attacker could try to inject a code for a different session; state nonce validation mitigates this.

## Residual Risks (Accepted / Not Fully Solvable)

Accepted for now:
- Same-user compromise: malware running as the same OS user can often read keychain entries or intercept process memory.
- User copy/paste of the auth URL into unsafe channels.
- If the OS credential store is unavailable, the CLI will fail unless the user explicitly allows insecure fallback.

Known architectural risk:
- Server uses Supabase service role key. Even with safe coding, mistakes can expand blast radius.
  Planned mitigation: strict allowlist/RPC-only access patterns and least-privilege deployment guidance.

## Security Requirements (Invariant Rules)

- Never accept JWTs without verifying signature + strict iss/aud/exp.
- Never log tokens, refresh tokens, or OAuth auth codes.
- Default to secure storage; no silent downgrade to plaintext or file-based secrets.
- Server endpoints must be explicit; avoid generic database passthrough endpoints.

## Operational Checklist

For developers/operators:
- Set `SUPABASE_URL` to the correct project.
- Keep `SUPABASE_SERVICE_ROLE_KEY` server-side only; never ship it to the CLI.
- Rotate keys if exposure is suspected.
- Avoid placing tokens in CI logs or shell history.

## How To Report Security Issues

Open a private security report to the maintainers. If private channels are not available,
open a minimal public issue that does not contain secrets and request a private follow-up.
