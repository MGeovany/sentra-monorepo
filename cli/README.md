# Sentra CLI

Sentra is a developer-first CLI for scanning, staging and pushing `.env*` files.

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
- Ensures the current `machine_id` is registered in the remote DB via `SENTRA_SERVER_URL`.

Usage:

- `sentra push`
