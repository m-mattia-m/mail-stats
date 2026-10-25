# Development, CI & releases

## Project layout

```
cmd/mail-stats/        main.go – only calls cli.Execute; version set via -ldflags
internal/cli/          cobra command tree (get, config, version), output formatting
internal/config/       config file, contexts, password resolution
internal/mailbox/      IMAP client (go-imap, LOGIN or XOAUTH2), folder classification, Collect
internal/oauth/        OAuth2 device-code login, token store + refresh, tokenCommand
internal/stats/        window + averages
internal/imaptest/     in-memory IMAP server for tests (plain, TLS, STARTTLS, XOAUTH2)
internal/oauthtest/    fake Microsoft identity platform (device code + refresh) for tests
.container/            Containerfile
.github/               workflows + Dependabot
docs/                  this documentation
```

## Tests and coverage

```bash
make test         # go test ./...
make cover        # race detector + coverage profile, like CI
make cover-html   # open the line-by-line report
make build        # bin/mail-stats
```

The tests need no network or real mailbox: `internal/imaptest` starts an
in-memory IMAP server on `127.0.0.1` (with a throw-away self-signed
certificate for the TLS modes), and the CLI tests run the real command tree
against it with a temporary config file.

CI fails if total coverage drops below **80%**. `cmd/` (a one-line `main`)
and `internal/imaptest` / `internal/oauthtest` (test fakes) are not counted, the same way
LinkShelf leaves out mocks.

## Commit messages

Releases are derived from [Conventional Commits](https://www.conventionalcommits.org/).
PRs are squash-merged and the PR title becomes the commit message, so the
**PR title** must follow the format (checked by `pr-title.yaml`):

| Title | Release |
|-------|---------|
| `fix: …`, `fix(deps): …` | patch (v1.0.0 → v1.0.1) |
| `feat: …` | minor (v1.0.0 → v1.1.0) |
| `feat!: …` or a `BREAKING CHANGE:` footer | major (v1.0.0 → v2.0.0) |
| `chore: …`, `ci: …`, `docs: …`, `test: …`, `refactor: …` | no release |

## Workflows

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| [`ci.yaml`](../.github/workflows/ci.yaml) | push to `main`, `feat/**`, `fix/**`, `chore/**`, `docs/**`, `refactor/**`, `style/**`, `test/**`, `develop/**`, `dependabot/**` | gofmt check, `go mod tidy` check, vet, build, tests with `-race`, coverage gate 80% (+ badge on `main`), multi-arch image build + smoke test (not pushed) |
| [`dependabot-auto-merge.yaml`](../.github/workflows/dependabot-auto-merge.yaml) | Dependabot PRs | enables GitHub auto-merge (squash) for patch/minor updates; GitHub merges once the required CI checks pass. Major updates are left for you. |
| [`release.yaml`](../.github/workflows/release.yaml) | every Tuesday 06:00 UTC, or manually (*Actions → Release → Run workflow*) | semantic-release dry run → if there is something to release: tests + coverage → tag `vX.Y.Z` + GitHub release with notes → binaries attached to the release → image published |
| [`image.yaml`](../.github/workflows/image.yaml) | a pushed tag `vX.Y.Z`, or called by `release.yaml` | builds and pushes `ghcr.io/m-mattia-m/mail-stats:vX.Y.Z` and `:latest` |
| [`pr-title.yaml`](../.github/workflows/pr-title.yaml) | PR opened/edited | PR title must be a Conventional Commit |

### The weekly cycle

1. **Monday** – Dependabot ([`dependabot.yml`](../.github/dependabot.yml)) opens
   grouped PRs for Go modules and container base images (`fix(deps): …`) and
   GitHub Actions (`chore(ci): …`).
2. CI runs on the Dependabot branch; when it's green, GitHub auto-merges the
   patch/minor PRs.
3. **Tuesday** – `release.yaml` runs. semantic-release looks at all commits
   since the last tag. Merged `fix(deps)` bumps (or your own `feat`/`fix`
   commits) → a new release. Only `chore(ci)` bumps → no release.

### Tags

Tags always have the form `v1.0.0` (`tagFormat: v${version}` in
[`.releaserc.json`](../.releaserc.json)). The first release is `v1.0.0`.
You can also push a tag yourself (`git tag v1.2.3 && git push origin v1.2.3`);
`image.yaml` then publishes the image for it (anything not matching
`vX.Y.Z` is rejected). Tags created by semantic-release don't trigger
workflows on their own (GitHub's `GITHUB_TOKEN` rule), which is why
`release.yaml` calls `image.yaml` directly.

## One-time GitHub setup

These settings are not in the repository and must be set once:

1. **Settings → General → Pull Requests**: enable **Allow auto-merge** (and
   squash merging).
2. **Settings → Branches** (or Rulesets): protect `main`, require status
   checks to pass, and select the checks **`ci`** and **`container`**.
   Without required checks, auto-merge would merge immediately.
3. **Settings → Actions → General → Workflow permissions**: *Read and write
   permissions* (semantic-release creates tags/releases, the coverage action
   pushes the badge to the `badges` branch).
4. After the first release: **Packages → mail-stats → Package settings** –
   make the image public if it should be pullable without logging in, and
   link it to the repository.
