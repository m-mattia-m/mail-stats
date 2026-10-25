# Container image

Every release publishes a multi-arch (amd64, arm64) OCI image:

```
ghcr.io/m-mattia-m/mail-stats:<version>   # e.g. v1.2.3
ghcr.io/m-mattia-m/mail-stats:latest
```

The image runs `mail-stats` as an unprivileged user (UID 1001). Its default
arguments are `get stats`, and it reads its config from
`/config/config.yaml` (`MAIL_STATS_CONFIG` is preset).

## Run it

Mount your config directory read-only and pass the password in:

```bash
podman run --rm \
  -v ~/.mail-stats:/config:ro \
  -e MAIL_STATS_PASSWORD \
  ghcr.io/m-mattia-m/mail-stats:latest

# other commands / flags
podman run --rm -v ~/.mail-stats:/config:ro -e MAIL_STATS_PASSWORD \
  ghcr.io/m-mattia-m/mail-stats:latest get folders --context work -o json
```

`docker run` works the same way. With rootless podman/docker the mounted file
must be readable by UID 1001 in the container – if you get a permission
error on `/config/config.yaml`, add `--userns=keep-id` (podman) or
`--user "$(id -u)"`.

## Passwords in the container

The image does **not** contain the 1Password CLI, so a context with
`passwordCommand: op read …` can't run inside it. Resolve the password on the
host and hand it in via `MAIL_STATS_PASSWORD`, which overrides every password
field:

```bash
# with op read
MAIL_STATS_PASSWORD="$(op read 'op://Private/Gmail IMAP/password')" \
  podman run --rm -v ~/.mail-stats:/config:ro -e MAIL_STATS_PASSWORD \
  ghcr.io/m-mattia-m/mail-stats:latest

# or with op run
MAIL_STATS_PASSWORD="op://Private/Gmail IMAP/password" op run -- \
  podman run --rm -v ~/.mail-stats:/config:ro -e MAIL_STATS_PASSWORD \
  ghcr.io/m-mattia-m/mail-stats:latest
```

`-e MAIL_STATS_PASSWORD` without `=value` copies the variable from your shell,
so the password doesn't appear in the process list. Contexts with
`passwordEnv` work the same way: pass that variable with `-e VARNAME`.

See [1Password](1password.md) for the `op` details.

## Build it yourself

```bash
make image      # podman build -f .container/Containerfile -t mail-stats:dev .
# or
docker build -f .container/Containerfile --build-arg VERSION=dev -t mail-stats:dev .
```

The Containerfile lives in [`.container/Containerfile`](../.container/Containerfile):
a Go builder stage cross-compiles a static binary, the runtime stage is
Alpine (it has a shell, which `passwordCommand` needs) with CA certificates.
