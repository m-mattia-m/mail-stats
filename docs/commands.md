# Commands

mail-stats is structured like `kubectl`: a verb (`get`, `config`) followed by
a resource or sub-command.

```
mail-stats
├── get
│   ├── stats      (aliases: stat, statistics)
│   └── folders    (aliases: folder, mailboxes)
├── config
│   ├── view
│   ├── get-contexts
│   ├── current-context
│   ├── use-context NAME
│   ├── set-context NAME
│   └── delete-context NAME
├── auth                 (OAuth2 accounts, e.g. Hotmail / Outlook.com)
│   ├── login [CONTEXT]
│   ├── logout [CONTEXT]
│   └── status
└── version
```

Every command has `--help`, e.g. `mail-stats get stats --help`.

On error, mail-stats prints `Error: <message>` to stderr and exits with code 1.

## Global flags

These work on every command:

| Flag | Default | Meaning |
|------|---------|---------|
| `--config PATH` | `$MAIL_STATS_CONFIG`, else `~/.mail-stats/config.yaml` | Config file to read/write |
| `--context NAME` | the current context | Use this context for this one command (does not change the current context) |
| `-o, --output FORMAT` | `table` | `table`, `json` or `yaml` |
| `--timeout DURATION` | `30s` | IMAP connect and per-command timeout, e.g. `90s`, `2m` |

---

## `get stats`

Connects to the account, reads every folder and prints the average number of
sent and received mails per day, week and month.

| Flag | Meaning |
|------|---------|
| `--since YYYY-MM-DD` | Only count mails on or after this day (00:00 local time). The window starts here instead of at your oldest mail. |
| `--until YYYY-MM-DD` | Only count mails on or before this day (until 23:59:59 local time). The window ends here instead of now. |
| `--contexts A,B` | Read several contexts instead of one (comma-separated or repeated). See [several accounts](#several-accounts-at-once). |
| `-A, --all-contexts` | Read every context in the config file. |

```bash
mail-stats get stats
mail-stats get stats --context work
mail-stats get stats --since 2025-01-01 --until 2025-12-31
mail-stats get stats -o json | jq '.stats.received.perWeek'
mail-stats get stats --contexts personal,work
mail-stats get stats -A
```

Table output:

```
CONTEXT:   personal (me@example.com @ imap.example.com)
PERIOD:    2015-03-02 – 2026-10-25 (4255 days)

DIRECTION   TOTAL   PER DAY   PER WEEK   PER MONTH   FIRST        LAST
sent        3120    0.73      5.13       22.30       2015-03-04   2026-10-24
received    48211   11.33     79.31      344.66      2015-03-02   2026-10-25
```

- **PERIOD** – the window all averages are based on (see [statistics](statistics.md)).
- **TOTAL** – number of mails in the window.
- **FIRST / LAST** – the oldest and newest mail of that direction in the window.

JSON / YAML output also contains every folder with its role and message count:

```json
{
  "context": "personal",
  "stats": {
    "from": "2015-03-02T08:12:44Z",
    "to": "2026-10-25T10:00:00+01:00",
    "days": 4255.07,
    "sent": {
      "total": 3120, "perDay": 0.73, "perWeek": 5.13, "perMonth": 22.30,
      "first": "2015-03-04T09:00:00Z", "last": "2026-10-24T18:30:00Z"
    },
    "received": { "total": 48211, "perDay": 11.33, "perWeek": 79.31, "perMonth": 344.66, "first": "…", "last": "…" }
  },
  "folders": [
    { "name": "INBOX", "role": "received", "messages": 40012 },
    { "name": "Sent", "role": "sent", "messages": 3120 },
    { "name": "Drafts", "role": "excluded", "messages": 0 }
  ]
}
```

### Several accounts at once

With `--contexts` or `-A`, every selected account is read **in parallel**
(passwords are fetched one after the other first, so a password manager asks
one thing at a time). The output then has one row pair per account plus a
**`(combined)`** row pair that adds all accounts together:

```
$ mail-stats get stats --contexts personal,work
CONTEXT      ACCOUNT                               PERIOD
personal     me@gmail.com @ imap.gmail.com         2015-03-02 – 2026-10-25 (4255 days)
work         me@work.com @ outlook.office365.com   2021-06-01 – 2026-10-25 (1972 days)
(combined)   2 accounts                            2015-03-02 – 2026-10-25 (4255 days)

CONTEXT      DIRECTION   TOTAL   PER DAY   PER WEEK   PER MONTH   FIRST        LAST
personal     sent        3120    0.73      5.13       22.30       2015-03-04   2026-10-24
personal     received    48211   11.33     79.31      344.66      2015-03-02   2026-10-25
work         sent        5410    2.74      19.20      83.50       2021-06-01   2026-10-24
work         received    30122   15.27     106.92     464.93      2021-06-01   2026-10-25
(combined)   sent        8530    2.00      14.03      61.01       2015-03-04   2026-10-24
(combined)   received    78333   18.41     128.86     560.34      2015-03-02   2026-10-25
```

- Each account is averaged over **its own** history (an account you opened
  last year isn't diluted by the years before it existed).
- `(combined)` is averaged over the window from the oldest mail of **any**
  selected account (or `--since`/`--until`).
- If two selected contexts log in to the same account (same username, host
  and port), mail-stats prints a warning on stderr: that mailbox's mails are
  then counted twice in `(combined)`.
- If any account fails (wrong password, unreachable server), the command fails
  with `context "<name>": …` and prints nothing.
- `--contexts`/`-A` can't be combined with `--context`, or with each other.
- Even with a single name (`--contexts personal`) you get this multi-account
  layout, which keeps scripts' output shape stable.

JSON/YAML with several contexts:

```json
{
  "contexts": [
    { "context": "personal", "stats": { "…": "same shape as above" }, "folders": [ … ] },
    { "context": "work",     "stats": { … }, "folders": [ … ] }
  ],
  "combined": { "from": "…", "to": "…", "days": 4255.07, "sent": { … }, "received": { … } }
}
```

Several accounts on the **same mail server** (e.g. `me@example.com` and
`shop@example.com` both on `imap.example.com`) are simply several contexts with
the same `host` and a different `username`:

```bash
mail-stats config set-context me   --host imap.example.com --username me@example.com \
  --password-command 'op read "op://Private/Mail me/password"'
mail-stats config set-context shop --host imap.example.com --username shop@example.com \
  --password-command 'op read "op://Private/Mail shop/password"'
mail-stats get stats --contexts me,shop
```

## `get folders`

Lists every folder on the server, how it is counted (`sent`, `received` or
`excluded`) and how many messages it holds. Excluded folders are not read, so
their count is shown as `-`. Use this to check the classification before
trusting the stats.

| Flag | Meaning |
|------|---------|
| `--contexts A,B` | List the folders of several contexts; adds a `CONTEXT` column |
| `-A, --all-contexts` | List the folders of every context |

```bash
mail-stats get folders
mail-stats get folders -o yaml
mail-stats get folders -A
# JSON with several contexts: [{"context": "personal", "folders": [...]}, ...]
mail-stats get folders --contexts personal,work -o json
```

```
NAME                ROLE       MESSAGES
INBOX               received   40012
Junk                received   812
Trash               received   6950
Projects/Customer   received   437
Sent                sent       3120
Drafts              excluded   -
```

---

## `config view`

Prints the whole config file. Plain-text `password` values are replaced by
`REDACTED`; `passwordEnv` and `passwordCommand` are shown as they are (they
are not secrets). Output is YAML by default, `-o json` works too.

```bash
mail-stats config view
```

## `config get-contexts`

Lists all contexts. The current one is marked with `*`. Ports and security
show their effective values (defaults filled in).

```
CURRENT   NAME       HOST                    PORT   USERNAME         SECURITY   AUTH
*         personal   imap.gmail.com          993    me@gmail.com     tls        password
          hotmail    outlook.office365.com   993    me@hotmail.com   tls        oauth2
```

## `config current-context`

Prints the name of the current context. Fails if none is set.

## `config use-context NAME`

Makes `NAME` the current context, i.e. the one used when you don't pass
`--context`. Fails if the context does not exist.

```bash
mail-stats config use-context work
```

## `config set-context NAME`

Creates the context `NAME`, or – if it already exists – changes **only the
fields whose flags you pass**. Everything else stays as it was. After the
change, the context must still have a host and username, otherwise nothing is
saved. If there is no current context yet, `NAME` becomes the current context.

| Flag | Config field | Meaning |
|------|--------------|---------|
| `--host HOST` | `host` | IMAP server host name |
| `--port N` | `port` | IMAP port; `0`/unset = 993 for `tls`, 143 for `starttls`/`none` |
| `--username USER` | `username` | IMAP login, usually your mail address |
| `--security MODE` | `security` | `tls` (default), `starttls` or `none` |
| `--password-env VAR` | `passwordEnv` | Read the password from environment variable `VAR` |
| `--password-command CMD` | `passwordCommand` | Run `CMD` with `sh -c`, use its output as the password (e.g. `op read …`) |
| `--insecure-skip-verify` | `insecureSkipVerify` | Don't verify the TLS certificate (only for local bridges with self-signed certs) |
| `--sent-folder NAME` | `sentFolders` | Count this folder as sent. Repeatable or comma-separated. **Replaces** the existing list. |
| `--exclude-folder NAME` | `excludeFolders` | Don't count this folder. Repeatable or comma-separated. **Replaces** the existing list. |
| `--auth MODE` | `auth` | `password` (default) or `oauth2` (Microsoft accounts, see [Microsoft](microsoft.md)) |
| `--oauth2-client-id ID` | `oauth2.clientId` | Application (client) ID of your Microsoft app registration |
| `--oauth2-tenant T` | `oauth2.tenant` | `consumers` (default: Hotmail/Outlook.com), `organizations`, `common` or a tenant ID |
| `--oauth2-scope S` | `oauth2.scopes` | Scopes to request; repeatable, replaces the list. Default: IMAP.AccessAsUser.All + offline_access |
| `--oauth2-token-command CMD` | `oauth2.tokenCommand` | Shell command that prints an access token, instead of `auth login` |

There is deliberately no `--password` flag: it would end up in your shell
history. If you really want a plain-text password, edit the config file (see
[Passwords](passwords.md)).

```bash
# create
mail-stats config set-context work \
  --host mail.example.com --username me@work.com \
  --password-command 'op read "op://Work/Work IMAP/password"'

# change one field
mail-stats config set-context work --port 1993

# switch from a command to an environment variable
mail-stats config set-context work --password-command "" --password-env WORK_MAIL_PW

# replace the folder lists (pass an empty value to clear them)
mail-stats config set-context work --sent-folder "Archive/Outgoing" --sent-folder "Sent Old"
mail-stats config set-context work --exclude-folder ""
```

Output: `Context "work" created.` or `Context "work" modified.`

## `config delete-context NAME`

Removes the context. If it was the current context, no context is current
afterwards – pick a new one with `use-context`.

---

## `auth login [CONTEXT]`

Signs in to an OAuth2 context (`auth: oauth2`) with Microsoft's device-code
flow: mail-stats prints a URL and a code, you open the URL in any browser,
enter the code and sign in with the mail account. The refresh token is stored
in `tokens/<context>.json` next to the config file (mode `0600`) and renewed
automatically by every later command. Without `CONTEXT` it uses `--context` or
the current context. Full walkthrough: [Microsoft accounts](microsoft.md).

```bash
mail-stats auth login hotmail
```

## `auth logout [CONTEXT]`

Deletes the stored token of the context. Fails if it isn't logged in.

## `auth status`

Lists every OAuth2 context and whether it is logged in:

```
NAME      USERNAME         TOKENS FROM    STATUS
hotmail   me@hotmail.com   auth login     logged in
work365   me@contoso.com   auth login     not logged in
other     me@outlook.com   tokenCommand   -
```

---

## `version`

Prints the version the binary was built with (the release tag, e.g. `v1.4.2`,
or `dev` for local builds).
