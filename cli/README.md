# Sentra CLI

Sentra is a developer-first CLI for scanning, staging and pushing `.env*` files.

## Setup

Set these env vars before logging in (in `.env` at repo root is fine):

- `SUPABASE_URL` (e.g. `https://<project>.supabase.co`)
- `SUPABASE_ANON_KEY`

Also, Supabase requires allowlisting redirect URLs.
Add this to your Supabase Dashboard → Auth → URL Configuration → Additional Redirect URLs:

- `http://localhost:53124/callback`

If the port is taken, set `SENTRA_AUTH_PORT` and allowlist the matching URL.

After login, Sentra stores local state under `~/.sentra/`:

- `~/.sentra/session.json` OAuth session (tokens)
- `~/.sentra/config.json` local config (`machine_id`, `user_id`)
- `~/.sentra/index.json` staged env files
- `~/.sentra/state.json` last known scan state

## Commands

### `sentra login`

Starts a browser-based login using Supabase Auth (GitHub provider) via PKCE.

- Prints a URL
- Waits for the callback
- Saves session + config to `~/.sentra/`

Usage:

- `sentra login`

### `sentra scan`

Scans `~/dev` for git repositories and detects `.env*` files.

Usage:

- `sentra scan`

### `sentra add`

Stages env files into the local index.

Usage:

- `sentra add .`
- `sentra add <path>`

### `sentra status`

Shows how many tracked env files changed since last snapshot.

Usage:

- `sentra status`

### `sentra commit`

Creates a local commit from staged env files.

Usage:

- `sentra commit -m "message"`

### `sentra log`

Shows local commit history.

Usage:

- `sentra log`

### `sentra push`

Pushes local commits to the remote.

- If there is no local session, it triggers `sentra login` automatically.

Usage:

- `sentra push`
