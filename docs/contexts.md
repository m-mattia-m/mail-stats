# Contexts

A **context** is one mail account: a name plus everything needed to connect
(host, port, username, security, where the password comes from) and optional
folder overrides. The idea is the same as in `kubectl`, where a context selects
the cluster you talk to.

All contexts live in one [config file](configuration.md). One of them can be
the **current context**: the account every command uses unless you say
otherwise.

## Which context does a command use?

1. The one named with `--context NAME`, if you pass it.
2. Otherwise the current context (`currentContext` in the config file).
3. If neither exists, the command fails with
   `no context selected: pass --context or run "mail-stats config use-context <name>"`.

`--context` only affects that single command. It never changes the current
context; only `config use-context` does.

## Lifecycle

```bash
# Create two accounts. The first one becomes the current context automatically.
mail-stats config set-context personal --host imap.gmail.com --username me@gmail.com \
  --password-command 'op read "op://Private/Gmail IMAP/password"'
mail-stats config set-context work --host outlook.office365.com --username me@work.com \
  --password-env WORK_MAIL_PW

mail-stats config get-contexts
# CURRENT   NAME       HOST                    PORT   USERNAME       SECURITY
# *         personal   imap.gmail.com          993    me@gmail.com   tls
#           work       outlook.office365.com   993    me@work.com    tls

mail-stats get stats                    # -> personal (current)
mail-stats get stats --context work     # -> work, just this once

mail-stats config use-context work      # switch permanently
mail-stats config current-context       # -> work

mail-stats config set-context work --port 1993   # update one field
mail-stats config delete-context work            # current context is now unset
mail-stats config use-context personal
```

## Several config files

Every command accepts `--config PATH`, and `$MAIL_STATS_CONFIG` sets the
default path. That lets you keep, for example, a separate config for a
container or a test account:

```bash
export MAIL_STATS_CONFIG=~/work/mail-stats.yaml
mail-stats config get-contexts
```

## Naming

Context names are free-form strings, compared exactly (case-sensitive).
Short names without spaces (`personal`, `work`, `gmail-old`) are easiest to type.
