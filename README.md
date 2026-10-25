# mail-stats

![coverage](https://raw.githubusercontent.com/m-mattia-m/mail-stats/badges/.badges/main/coverage.svg)

A kubectl-style CLI that connects to an IMAP account, reads every folder and
shows how many mails you **send** and **receive** on average per day, week and
month over your whole mail history.

```
$ mail-stats get stats
CONTEXT:   personal (me@example.com @ imap.example.com)
PERIOD:    2015-03-02 – 2026-10-25 (4255 days)

DIRECTION   TOTAL   PER DAY   PER WEEK   PER MONTH   FIRST        LAST
sent        3120    0.73      5.13       22.30       2015-03-04   2026-10-24
received    48211   11.33     79.31      344.66      2015-03-02   2026-10-25
```

## Documentation

Full documentation is in [`docs/`](docs/README.md):
[getting started](docs/getting-started.md) ·
[commands](docs/commands.md) ·
[contexts](docs/contexts.md) ·
[config file](docs/configuration.md) ·
[passwords](docs/passwords.md) ·
[1Password CLI](docs/1password.md) ·
[how stats are calculated](docs/statistics.md) ·
[container](docs/container.md) ·
[development & releases](docs/development.md)

## How mails are counted

| Folder                                                        | Counted as |
|---------------------------------------------------------------|------------|
| Sent folders: `\Sent` attribute (RFC 6154), common names (`Sent`, `Sent Items`, `Gesendet`, …) or `sentFolders` from the context | sent |
| Every other folder: INBOX, Junk, Trash, Archive, custom folders | received |
| Drafts, non-selectable folders and virtual folders that only mirror other folders (Gmail's *All Mail*, *Starred*, *Important*), plus `excludeFolders` | not counted |

- A mail's date is its IMAP `INTERNALDATE` (when the server received/stored it).
- The averages use one window for both directions: from the oldest mail
  (or `--since`) to now (or `--until`). Total ÷ days, ÷ (days / 7) and
  ÷ (days / 30.44). A period shorter than one week/month counts as one, so a
  few days of history are not extrapolated.
- `mail-stats get folders` shows how each folder was classified.

## Install

```bash
go install github.com/m-mattia-m/mail-stats/cmd/mail-stats@latest
```

Or download a binary from the [releases](https://github.com/m-mattia-m/mail-stats/releases),
or use the container image (see below).

## Usage

```bash
# Create a context (the first one becomes the current context)
mail-stats config set-context personal \
  --host imap.example.com --username me@example.com \
  --password-command "op read op://Private/imap/password"

mail-stats config set-context work --host outlook.office365.com \
  --username me@work.com --password-env WORK_MAIL_PASSWORD

mail-stats config get-contexts
mail-stats config use-context work
mail-stats config current-context
mail-stats config view
mail-stats config delete-context work

# Stats
mail-stats get stats
mail-stats get stats --context personal --since 2024-01-01 --until 2024-12-31
mail-stats get stats -o json      # or -o yaml (includes the per-folder counts)
mail-stats get folders

mail-stats version
```

Global flags: `--config`, `--context`, `-o/--output table|json|yaml`, `--timeout`.

### Config file

`~/.mail-stats/config.yaml` (override with `--config` or `$MAIL_STATS_CONFIG`),
written with mode `0600`:

```yaml
currentContext: personal
contexts:
  - name: personal
    host: imap.example.com
    port: 993                 # default 993 for tls, 143 for starttls/none
    username: me@example.com
    security: tls             # tls (default) | starttls | none
    passwordCommand: op read op://Private/imap/password
    # passwordEnv: MAIL_PASSWORD
    # password: plain-text    # discouraged
    sentFolders: [Archive/Outgoing]
    excludeFolders: [Newsletters]
    insecureSkipVerify: false
```

Password precedence: `$MAIL_STATS_PASSWORD` → `passwordCommand` → `passwordEnv` → `password`.

For Gmail, enable IMAP and use an [app password](https://support.google.com/accounts/answer/185833).

### Container

```bash
podman run --rm -v ~/.mail-stats:/config:ro -e MAIL_STATS_PASSWORD \
  ghcr.io/m-mattia-m/mail-stats:latest get stats
```

## Development

```bash
make test        # go test ./...
make cover       # tests + coverage like CI (80% gate)
make cover-html
make build       # bin/mail-stats
make image       # local container build with podman
```

The tests run against an in-memory IMAP server (`internal/imaptest`), so no
network or real mailbox is needed.

## CI / Release

- **ci.yaml** – on every push: gofmt, `go mod tidy`, vet, build, tests with
  race detector, coverage gate (80%, badge on `main`), multi-arch image build.
- **dependabot.yml** – weekly (Monday) grouped updates for Go modules, base
  images (`fix(deps)` → patch release) and GitHub Actions (`chore(ci)` → no release).
- **dependabot-auto-merge.yaml** – enables auto-merge on Dependabot
  patch/minor PRs; GitHub merges once CI is green.
- **release.yaml** – every Tuesday (and manually): semantic-release decides
  from the Conventional Commits whether there is something to release, runs the
  tests, creates the `vX.Y.Z` tag + GitHub release, uploads binaries and
  publishes the image.
- **image.yaml** – builds and pushes `ghcr.io/m-mattia-m/mail-stats:<tag>` and
  `:latest` for every `vX.Y.Z` tag (also called by release.yaml).
- **pr-title.yaml** – PR titles must be Conventional Commits (they become the
  squash commit message semantic-release reads).
