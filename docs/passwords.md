# Passwords

Microsoft accounts (Hotmail, Outlook.com, Microsoft 365) don't use passwords
at all – see [Microsoft accounts](microsoft.md). Everything below applies to
contexts with `auth: password` (the default).

mail-stats needs your IMAP password every time it connects. It never stores
one by itself – you choose where it comes from.

## The four sources, and which one wins

When a command connects, the password is taken from the **first** source that
applies:

| # | Source | Set with | Good for |
|---|--------|----------|----------|
| 1 | `MAIL_STATS_PASSWORD` environment variable | `export MAIL_STATS_PASSWORD=…` | One-off runs, containers, `op run` |
| 2 | `passwordCommand` of the context | `--password-command CMD` | **Recommended:** a password manager such as [1Password](1password.md) |
| 3 | `passwordEnv` of the context | `--password-env VAR` | Secrets already exported in your shell / CI |
| 4 | `password` of the context | editing the config file | Throw-away test accounts only |

If none applies the command fails with
`context "x" has no password: set passwordEnv, passwordCommand or MAIL_STATS_PASSWORD`.

Note that a failure at a higher level does **not** fall through to the next
one: if `passwordCommand` fails, or the variable named in `passwordEnv` is
empty, you get an error instead of silently using another password.

## `passwordCommand`

The command is run with `sh -c "<command>"`, so quoting, pipes and `$VARS`
work like in a shell script. mail-stats uses what the command prints on
**stdout**, with trailing newlines removed. The command's **stderr and stdin
stay connected to your terminal**, so a password manager can ask you to
unlock it or sign in, and its error messages are shown to you.

If the command exits with a non-zero status, mail-stats stops with
`context "x": passwordCommand failed: exit status N` (the command's own
error message is printed just above that).

Examples:

```bash
# 1Password (see 1password.md for details)
--password-command 'op read "op://Private/Gmail IMAP/password"'

# macOS Keychain (add it first: security add-generic-password -s mail-stats-gmail -a me@gmail.com -w)
--password-command 'security find-generic-password -s mail-stats-gmail -w'

# pass (passwordstore.org) – first line of the entry
--password-command 'pass show mail/gmail | head -n1'

# Bitwarden CLI
--password-command 'bw get password "Gmail IMAP"'
```

Quoting tip: wrap the whole command in **single** quotes on the command line
and use **double** quotes inside it, as above. In the YAML file you can write
it unquoted (`passwordCommand: op read "op://Private/Gmail IMAP/password"`).

## `passwordEnv`

```bash
mail-stats config set-context work --password-env WORK_MAIL_PW
export WORK_MAIL_PW='…'      # e.g. in a shell profile you don't commit
mail-stats get stats --context work
```

The config only stores the variable's **name**, never its value.

## `MAIL_STATS_PASSWORD`

Overrides the password of whatever context is used, without touching the
config. Handy for scripts and the [container image](container.md):

```bash
MAIL_STATS_PASSWORD="$(op read 'op://Private/Gmail IMAP/password')" mail-stats get stats
```

Because it applies to every context, set it only for a single command (as
above) rather than exporting it in your shell. With `--contexts` / `-A` it is
used for **all** selected accounts – only useful if they really share one
password; otherwise give each context its own `passwordCommand` or
`passwordEnv`.

## Several contexts in one run

With `--contexts` / `-A`, the passwords of all selected contexts are resolved
**one after the other, before** any mailbox is opened. So 1Password (or any
other `passwordCommand`) is asked once per context, never in parallel, and a
missing password fails the command before anything is read.

## `password` (plain text)

```yaml
contexts:
  - name: test
    host: 127.0.0.1
    username: test
    password: test
```

There is no command-line flag for this on purpose (it would end up in your
shell history). The config file is created with mode `0600`, and
`config view` shows the value as `REDACTED`, but the password is still stored
unencrypted on disk – avoid it for real accounts.

## App passwords

Many providers don't accept your normal account password over IMAP once
2-step verification is on. Create an app-specific password and store **that**
(e.g. in 1Password):

- Gmail: <https://support.google.com/accounts/answer/185833> (and enable IMAP in Gmail's settings)
- iCloud: <https://support.apple.com/en-us/102654>
- Yahoo: "Generate app password" in the account security settings
