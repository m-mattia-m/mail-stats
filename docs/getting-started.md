# Getting started

## 1. Install

Pick one:

```bash
# With Go
go install github.com/m-mattia-m/mail-stats/cmd/mail-stats@latest

# From source
git clone https://github.com/m-mattia-m/mail-stats && cd mail-stats
make build            # -> bin/mail-stats
```

Prebuilt binaries for Linux, macOS and Windows (amd64/arm64) are attached to
every [GitHub release](https://github.com/m-mattia-m/mail-stats/releases), and
there is a [container image](container.md).

Check it works:

```bash
mail-stats version
```

## 2. Find your IMAP settings

You need three things from your mail provider:

| Provider | Host | Port / security | Password |
|----------|------|-----------------|----------|
| Gmail | `imap.gmail.com` | 993 / tls | an [app password](https://support.google.com/accounts/answer/185833) (needs 2-step verification), IMAP enabled in Gmail settings |
| iCloud | `imap.mail.me.com` | 993 / tls | an [app-specific password](https://support.apple.com/en-us/102654) |
| Hotmail / Outlook.com / Live / Microsoft 365 | `outlook.office365.com` | 993 / tls | **no password** – OAuth2 sign-in, see [Microsoft accounts](microsoft.md) |
| Infomaniak | `mail.infomaniak.com` | 993 / tls | your mail password |
| Proton (via Proton Mail Bridge) | `127.0.0.1` | 1143 / starttls | the Bridge password; add `--insecure-skip-verify` (self-signed cert) |
| Anything else | see your provider's "IMAP settings" help page | usually 993 / tls | |

## 3. Store the password

Microsoft accounts don't use a password: follow [Microsoft accounts](microsoft.md)
instead of steps 3 and 4, then continue with step 5.

Do **not** put the password into the config file in plain text. Either keep it
in 1Password (recommended, see [1Password CLI](1password.md)) or in an
environment variable (see [Passwords](passwords.md)).

## 4. Create a context

A *context* is one mail account (see [Contexts](contexts.md)). With 1Password:

```bash
mail-stats config set-context personal \
  --host imap.gmail.com \
  --username me@gmail.com \
  --password-command 'op read "op://Private/Gmail IMAP/password"'
```

or with an environment variable:

```bash
mail-stats config set-context personal \
  --host imap.gmail.com \
  --username me@gmail.com \
  --password-env GMAIL_APP_PASSWORD
export GMAIL_APP_PASSWORD='abcd efgh ijkl mnop'
```

The first context you create automatically becomes the current one.

## 5. Look at your folders, then your stats

```bash
mail-stats get folders   # check that every folder is classified the way you expect
mail-stats get stats
```

If a folder is classified wrong (e.g. your client stores sent mail in
`Archive/Outgoing`), fix it with `--sent-folder` or `--exclude-folder`, see
[How the statistics are calculated](statistics.md#adjusting-the-classification).

Reading a large mailbox can take a while: every message's date is fetched once
per run. If you hit the default 30 second timeout on a slow server, use
`--timeout 2m`.
