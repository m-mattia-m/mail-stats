# Configuration file

## Location

In order of precedence:

1. `--config PATH` on the command line
2. the environment variable `MAIL_STATS_CONFIG`
3. `~/.mail-stats/config.yaml`

If the file does not exist, mail-stats behaves as if it were empty; the first
`config set-context` creates it (and the directory) for you. The file is
written with permissions `0600` (only you can read it) and the directory with
`0700`, because it can contain a password.

You can edit the file by hand or manage it entirely with the
[`config` commands](commands.md#config-view) – both work. Note that mail-stats
rewrites the whole file when a `config` command saves it, so comments you add
by hand are lost.

## Full example

```yaml
# The context used when --context is not passed.
currentContext: personal

contexts:
  - name: personal
    host: imap.gmail.com
    port: 993
    username: me@gmail.com
    security: tls
    passwordCommand: op read "op://Private/Gmail IMAP/password"
    # Gmail: "All Mail", "Starred" and "Important" are excluded automatically.

  - name: work
    host: mail.example.com
    username: me@work.com
    passwordEnv: WORK_MAIL_PW
    sentFolders:
      - Archive/Outgoing
    excludeFolders:
      - Newsletters
      - Conversation History

  - name: proton
    host: 127.0.0.1
    port: 1143
    username: me@proton.me
    security: starttls
    insecureSkipVerify: true      # Proton Mail Bridge uses a self-signed certificate
    passwordCommand: op read "op://Private/Proton Bridge/password"

  - name: hotmail                 # Microsoft: OAuth2 instead of a password
    host: outlook.office365.com
    username: me@hotmail.com
    auth: oauth2
    oauth2:
      clientId: 1b2c3d4e-0000-0000-0000-000000000000
```

Tokens of `auth: oauth2` contexts are stored in a `tokens/` directory next to
the config file (see [Microsoft](microsoft.md#where-tokens-are-stored)).

## Fields

### Top level

| Field | Type | Meaning |
|-------|------|---------|
| `currentContext` | string | Name of the default context. Empty = none, then `--context` is required. |
| `contexts` | list | The contexts, see below. |

### Per context

| Field | Type | Required | Default | Meaning |
|-------|------|----------|---------|---------|
| `name` | string | yes | | Unique name of the context |
| `host` | string | yes | | IMAP server host name or IP |
| `username` | string | yes | | IMAP login name, usually the mail address |
| `port` | int | no | 993 for `tls`, 143 for `starttls`/`none` | IMAP port (1–65535) |
| `security` | string | no | `tls` | `tls`: TLS from the first byte (port 993). `starttls`: plain connection upgraded to TLS with STARTTLS (port 143). `none`: no encryption – the password is sent in clear text; only for servers on `127.0.0.1`. |
| `passwordCommand` | string | one of the password fields | | Shell command whose output is the password, e.g. `op read "op://…"`. See [Passwords](passwords.md) and [1Password](1password.md). |
| `passwordEnv` | string | | | Name of an environment variable that holds the password |
| `password` | string | | | Plain-text password. Discouraged. |
| `insecureSkipVerify` | bool | no | `false` | Accept any TLS certificate. Only for local bridges with self-signed certificates – it makes the connection open to interception. |
| `sentFolders` | list of strings | no | `[]` | Full folder names (as shown by `get folders`) to count as **sent**, in addition to the automatically detected ones. Case-insensitive. |
| `excludeFolders` | list of strings | no | `[]` | Full folder names not to count at all. Case-insensitive. Wins over `sentFolders` and the automatic detection. |
| `auth` | string | no | `password` | `password`: IMAP LOGIN with the password from the fields above. `oauth2`: IMAP AUTHENTICATE XOAUTH2 with an access token – required for Microsoft accounts, see [Microsoft](microsoft.md). |
| `oauth2.clientId` | string | for `auth login` | | Application (client) ID of your Microsoft app registration |
| `oauth2.tenant` | string | no | `consumers` | `consumers` (Hotmail/Outlook.com/Live), `organizations`, `common`, or a tenant ID/domain |
| `oauth2.scopes` | list of strings | no | `https://outlook.office.com/IMAP.AccessAsUser.All`, `offline_access` | OAuth2 scopes to request |
| `oauth2.tokenCommand` | string | instead of `clientId` | | Shell command printing an access token (external token tool); replaces `auth login` |
| `oauth2.provider` | string | no | `microsoft` | Identity provider; only `microsoft` is built in |

With `auth: oauth2` the password fields are ignored, and either
`oauth2.clientId` or `oauth2.tokenCommand` must be set.

Folder names are the full path including the parent, with the server's
separator: `Archive/Outgoing` on most servers, `INBOX.Archive.Outgoing` on
Dovecot/Courier setups that use `.`. Copy them from `mail-stats get folders`.

How the automatic detection works is explained in
[How the statistics are calculated](statistics.md).

## Validation

Before connecting, and before `set-context` saves, the context is checked:
`name`, `host` and `username` must be set, `security` must be one of the
three modes and `port` must be in range. Errors look like:

```
Error: context "work" is missing: host, username
Error: context "work": unknown security "ssl" (want tls, starttls or none)
```

## Environment variables

| Variable | Effect |
|----------|--------|
| `MAIL_STATS_CONFIG` | Default config file path |
| `MAIL_STATS_PASSWORD` | Password for whichever context is used; overrides every password field (see [Passwords](passwords.md)) |
| the one named in `passwordEnv` | Password of that context |
