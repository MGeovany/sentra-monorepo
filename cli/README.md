# Sentra CLI

Sentra is a developer-first CLI for scanning, staging and pushing `.env*` files.

## Commands

### `sentra login`

Starts a browser-based login flow.

- Prints a login URL
- Waits for the callback
- Persists local session and machine identity

Usage:

- `sentra login`

### `sentra scan`

Scans a configured root for git repositories and detects `.env*` files.

On first run it prompts you for a scan root (defaults to `~/dev`).

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

### `sentra overview`

Shows a per-project card view with useful metadata (env count, staged, changed, latest modified).

Usage:

- `sentra overview`

### `sentra sync`

Downloads the latest env files from the remote and writes them into local repos under the configured scan root.

Usage:

- `sentra sync`

### `sentra history`

Lists remote commit history across all projects.

Usage:

- `sentra history`

### `sentra wipe`

Deletes ALL local Sentra state (logout + local commits + configs) and clears relevant keychain entries.

Usage:

- `sentra wipe`

### `sentra commit`

Creates a local commit from staged env files.

Usage:

- `sentra commit -m "message"`

### `sentra log`

Shows local commit history.

Usage:

- `sentra log` (pending only)
- `sentra log all`
- `sentra log pushed`

Manage local commits:

- `sentra log rm <id>`
- `sentra log clear`
- `sentra log prune <id|all>`
- `sentra log verify`

### `sentra push`

Pushes local commits to the remote.

- If there is no local session, it triggers `sentra login` automatically.
- Ensures the current machine identity is registered remotely.

Usage:

- `sentra push`
