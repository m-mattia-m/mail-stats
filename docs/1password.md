# Using the 1Password CLI (`op`)

With 1Password your IMAP password never touches the mail-stats config file.
The context only stores a **command** that asks 1Password for the password
each time mail-stats connects:

```yaml
passwordCommand: op read "op://Private/Gmail IMAP/password"
```

This page explains everything needed to get there: installing `op`, signing
in, storing the password, building the `op://…` reference and testing it.
(Commands checked against `op` 2.21; `op --version` shows yours.)

## How it fits together

```
mail-stats get stats
  └─ reads the context → passwordCommand: op read "op://Private/Gmail IMAP/password"
       └─ runs: sh -c 'op read "op://Private/Gmail IMAP/password"'
            └─ op asks the 1Password app (Touch ID / system password if locked)
            └─ op prints the password on stdout
  └─ mail-stats logs in to IMAP with it, keeps it in memory only
```

## 1. Install the CLI

```bash
# macOS
brew install 1password-cli

op --version
```

Other systems: <https://developer.1password.com/docs/cli/get-started/#install>.

## 2. Sign in

There are two ways. Use **A** on your own computer.

### A. Through the 1Password desktop app (recommended)

`op` borrows the desktop app's sign-in, so you unlock with Touch ID instead of
typing your account password.

1. Open the 1Password app.
2. **Settings → Security**: turn on **Touch ID** (or Windows Hello / your Linux
   system authentication).
3. **Settings → Developer**: turn on **Integrate with 1Password CLI**.
4. In a terminal:

   ```bash
   op signin        # prompts for Touch ID; does nothing if already signed in
   op whoami        # shows the account you are signed in to
   op vault list    # lists the vaults you can read
   ```

After that, every `op` command (including the one mail-stats runs) works as
long as the app is unlocked or you approve the Touch ID prompt. The approval is
per terminal session and is remembered for a while, so you won't be asked on
every run.

**Several 1Password accounts** (e.g. personal + work): `op` uses, in order,
the `--account` flag, the `OP_ACCOUNT` environment variable, or the account
you last signed in to. To make mail-stats always use the right one, put the
flag into the command:

```bash
op account list     # shows the account URLs / shorthands
--password-command 'op read --account my.1password.com "op://Private/Gmail IMAP/password"'
```

### B. Without the desktop app (servers, SSH sessions)

```bash
op account add --address my.1password.com --email me@example.com
eval "$(op signin)"     # asks for your account password (and secret key the first time)
```

`op signin` prints an `export OP_SESSION_…=…` line; `eval` sets that variable
in your current shell, and mail-stats passes it on to the `op` command it
runs. The session expires after 30 minutes of inactivity – then run
`eval "$(op signin)"` again.

### C. Fully unattended (cron, CI): service accounts

A [service account](https://developer.1password.com/docs/service-accounts/)
authenticates with a token instead of a person:

```bash
export OP_SERVICE_ACCOUNT_TOKEN='ops_…'
op read "op://Mail Automation/Gmail IMAP/password"
```

Service accounts **cannot** read your built-in *Private* / *Personal* vault.
Put the item into a dedicated vault (e.g. *Mail Automation*) and grant the
service account read access to only that vault. Keep the token itself secret
(it is as powerful as the vault access you gave it).

## 3. Store the IMAP password in 1Password

If it isn't in 1Password yet, create it in the app (that way the password is
never typed into a terminal or your shell history):

1. **New Item → Login** (or **Password**).
2. Title: e.g. `Gmail IMAP`.
3. Username: your mail address.
4. Password: the IMAP / app password (see [app passwords](passwords.md#app-passwords)).
5. Save it in the vault you want, e.g. **Private**.

If your normal login item for the provider already holds the right password
(no app password needed), you can just reference that item.

## 4. Build the secret reference

`op read` takes a **secret reference**:

```
op://<vault>/<item>/<field>
op://<vault>/<item>/<section>/<field>      # if the field is inside a section
```

| Part | What to put there | Example |
|------|-------------------|---------|
| vault | Vault name (or ID) | `Private`, `Personal`, `Work` |
| item | Item title (or ID) | `Gmail IMAP` |
| section | Only if the field is in a named section of the item | `IMAP` |
| field | Field label (or ID). For Login and Password items the password field is called `password`. | `password`, `app-password` |

Examples:

```
op://Private/Gmail IMAP/password
op://Work/Outlook/IMAP/app-password          # custom field "app-password" in section "IMAP"
op://Private/abcdefghijklmnopqrstuvwxyz/password   # item given by ID
```

Rules worth knowing:

- Names may contain letters, digits, spaces, `-` and `_`. **Any other special
  character** (e.g. `Mail (private)`, `me@gmail.com`, `Gmail/IMAP`) is not
  allowed in a reference – use the **ID** of that part instead (see below).
- If any part contains a space, quote the whole reference:
  `op read "op://Private/Gmail IMAP/password"`.
- If two items in the vault have the same title, `op` can't tell them apart –
  rename one or use the item ID.

### Easiest: copy it from the app

In the 1Password app, open the item, click the **▾** next to the password
field and choose **Copy Secret Reference**. You get a ready-to-use
`op://…` string (the app uses IDs automatically where names wouldn't work).

### Or find it on the command line

```bash
# Which vaults are there?
op vault list

# Which items are in a vault?
op item list --vault Private
op item list --vault Private --categories Login,Password

# Which fields (labels, sections) does an item have? Values are concealed.
op item get "Gmail IMAP" --vault Private

# The item's ID (for names with special characters)
op item get "Mail (private)" --vault Private --format json | jq -r .id
```

In the `op item get` output, the field labels are listed under **Fields**;
fields inside a section are listed under that section's name.

## 5. Test the reference

Check it without printing the password to your screen:

```bash
op read "op://Private/Gmail IMAP/password" > /dev/null && echo "reference works"

# or show only its length
op read -n "op://Private/Gmail IMAP/password" | wc -c
```

## 6. Configure the context

```bash
mail-stats config set-context personal \
  --host imap.gmail.com \
  --username me@gmail.com \
  --password-command 'op read "op://Private/Gmail IMAP/password"'

mail-stats get stats
```

Single quotes outside, double quotes inside, so the space in the item name
survives. The resulting config entry:

```yaml
- name: personal
  host: imap.gmail.com
  username: me@gmail.com
  passwordCommand: op read "op://Private/Gmail IMAP/password"
```

To change an existing context from another password source to 1Password:

```bash
mail-stats config set-context personal --password-env "" \
  --password-command 'op read "op://Private/Gmail IMAP/password"'
```

## Alternative: `op run` instead of `passwordCommand`

`op run` starts a program with environment variables whose values are secret
references resolved by 1Password. Combined with `MAIL_STATS_PASSWORD` (which
overrides every context's password) nothing about 1Password has to be in the
config:

```bash
MAIL_STATS_PASSWORD="op://Private/Gmail IMAP/password" op run -- mail-stats get stats
```

or one-off, without `op run`:

```bash
MAIL_STATS_PASSWORD="$(op read 'op://Private/Gmail IMAP/password')" mail-stats get stats
```

This is also the way to use 1Password with the [container image](container.md),
which does not contain `op`.

## Troubleshooting

When `op` fails, mail-stats shows `op`'s own message and then
`passwordCommand failed: exit status 1`. Common messages (the exact wording
differs between `op` versions):

| `op` says (roughly) | Meaning / fix |
|-----------|---------------|
| `command not found: op` / `sh: op: not found` | The CLI isn't installed or not on `PATH`. Run `which op`. If you start mail-stats from cron/launchd, use the full path, e.g. `/opt/homebrew/bin/op read …`. |
| `account is not signed in` / `You are not currently signed in` | Run `op signin` (A) or `eval "$(op signin)"` (B). With the app integration, check the Developer setting is on and the app is running. |
| `authorization prompt dismissed` / `authorization timeout` | You cancelled or missed the Touch ID prompt – run the command again. |
| `"Gmail IMAP" isn't an item in the "Private" vault` | Typo in the item title, or wrong vault. `op item list --vault Private`. |
| `isn't a vault in this account` | Vault name wrong (`op vault list`) or the wrong 1Password account is used (`op whoami`, `--account`). |
| `More than one item matches` | Two items share the title – rename one or use the item ID. |
| `isn't a field in the item` | Wrong field label or missing section: `op item get "<item>" --vault <vault>` lists them. |
| `invalid secret reference` / `invalid character in secret reference` | A part contains a character other than letters, digits, space, `-`, `_` – use IDs (Copy Secret Reference does this for you). |
| Login to IMAP fails (`login as … failed`) but `op read` works | The stored password is wrong for IMAP – usually an app password is required. |

Debug step by step:

```bash
op whoami                                           # signed in? which account?
op read "op://Private/Gmail IMAP/password" >/dev/null && echo ok
mail-stats config view                              # is passwordCommand exactly what you expect?
MAIL_STATS_PASSWORD="$(op read 'op://Private/Gmail IMAP/password')" mail-stats get folders
```

## Security notes

- The password is only held in memory by mail-stats for the duration of the
  run; it is never written to disk or printed.
- The config file only contains the reference (`op://…`), which is not a
  secret by itself – it is safe to show with `config view`.
- Anything that can run commands as your user while 1Password is unlocked
  could run the same `op read`. That's inherent to CLI access; approve Touch ID
  prompts only for commands you started.

Further reading: [1Password CLI docs](https://developer.1password.com/docs/cli/),
[secret reference syntax](https://developer.1password.com/docs/cli/secret-reference-syntax/),
[app integration](https://developer.1password.com/docs/cli/app-integration/).
