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

```bash
mail-stats get stats
mail-stats get stats --context work
mail-stats get stats --since 2025-01-01 --until 2025-12-31
mail-stats get stats -o json | jq '.stats.received.perWeek'
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

## `get folders`

Lists every folder on the server, how it is counted (`sent`, `received` or
`excluded`) and how many messages it holds. Excluded folders are not read, so
their count is shown as `-`. Use this to check the classification before
trusting the stats.

```bash
mail-stats get folders
mail-stats get folders -o yaml
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
CURRENT   NAME       HOST                    PORT   USERNAME         SECURITY
*         personal   imap.gmail.com          993    me@gmail.com     tls
          work       outlook.office365.com   993    me@work.com      tls
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

There is deliberately no `--password` flag: it would end up in your shell
history. If you really want a plain-text password, edit the config file (see
[Passwords](passwords.md)).

```bash
# create
mail-stats config set-context work \
  --host outlook.office365.com --username me@work.com \
  --password-command 'op read "op://Work/Outlook IMAP/password"'

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

## `version`

Prints the version the binary was built with (the release tag, e.g. `v1.4.2`,
or `dev` for local builds).
