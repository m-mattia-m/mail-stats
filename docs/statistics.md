# How the statistics are calculated

## What is read

mail-stats lists every folder of the account (IMAP `LIST "" "*"`), sorts each
one into a role, and for every non-excluded folder fetches the
**INTERNALDATE** of each message. INTERNALDATE is the time the server received
the mail (for sent mail: when your client stored the copy in the Sent folder).
Only dates are fetched – no headers, bodies or attachments – and every folder
is opened read-only, so nothing is changed and no mail is marked as read.

## Folder roles

| Role | Counted as | Folders |
|------|-----------|---------|
| `sent` | sent mails | see below |
| `received` | received mails | every folder that is neither sent nor excluded: INBOX, Junk/Spam, Trash, Archive, and all your custom folders |
| `excluded` | not counted | see below |

The rules are applied **in this order**, the first match wins:

1. Listed in the context's `excludeFolders` → **excluded**
2. Listed in the context's `sentFolders` → **sent**
3. The server flags the folder as (RFC 6154 special-use / RFC 3501 attributes):
   - `\Noselect` / `\NonExistent` (a folder that only groups subfolders) → **excluded**
   - `\Drafts` → **excluded** (drafts were never sent or received)
   - `\All` (Gmail *All Mail*), `\Flagged` (Gmail *Starred*), `\Important` (Gmail *Important*) → **excluded**, because these virtual folders only mirror mails that are already counted in other folders
4. The server flags the folder as `\Sent` → **sent**
5. The folder's last path component (case-insensitive) is a well-known sent
   folder name → **sent**:
   `Sent`, `Sent Items`, `Sent Messages`, `Sent Mail`, `Gesendet`,
   `Gesendete Elemente`, `Gesendete Objekte`, `Envoyés`, `Éléments envoyés`,
   `Messages envoyés`, `Inviata`, `Posta inviata`, `Enviados`, `Elementos enviados`
6. The last path component is a well-known drafts name → **excluded**:
   `Drafts`, `Draft`, `Entwürfe`, `Brouillons`, `Bozze`, `Borradores`
7. Everything else → **received**

So `INBOX.Sent`, `[Gmail]/Sent Mail` and `Archive/2019/Sent` are all detected
as sent, while `Junk` and `Trash` count as received (they contain mails you
received, even if you didn't want them).

### Adjusting the classification

Always check with `mail-stats get folders` first. Then:

```bash
# A folder with sent mail that isn't detected
mail-stats config set-context personal --sent-folder "Archive/Outgoing"

# Folders you don't want in the stats (e.g. a newsletter folder, a migrated copy)
mail-stats config set-context personal --exclude-folder "Newsletters" --exclude-folder "Old Server Import"
```

Both flags take full folder names exactly as `get folders` prints them
(compared case-insensitively) and replace the previous list.

### Things to be aware of

- A sent mail you later deleted is in **Trash**, so it counts as received.
- If your client files sent mail into other folders (e.g. "move replies to
  the conversation folder"), those copies count as received unless you
  exclude or mark those folders.
- A mail stored in two folders (copied, not moved) is counted twice. The
  virtual Gmail folders that would cause this on a large scale are excluded
  automatically.
- Imported/migrated mails have the INTERNALDATE of the import, unless the
  import tool preserved the original date.

## The averages

Sent and received use **the same time window**, so their numbers are directly
comparable:

- **Start** (`from`): `--since` if given, otherwise the oldest mail of either
  direction.
- **End** (`to`): end of the `--until` day if given, otherwise now.
- Mails outside the window are ignored.

With `days = (end − start)` in days (fractional):

```
per day   = total / days
per week  = total / (days / 7)
per month = total / (days / 30.436875)     # 365.2425 / 12, the average Gregorian month
```

With several contexts (`--contexts` / `-A`), every account gets its own
window as described above, and the `(combined)` numbers use the window from
the oldest mail of any selected account with all their mails added together.

Each period count is **at least 1**: with only 3 days of history,
`per week = total / 1`, not `total / 0.43` – mail-stats doesn't extrapolate a
weekly or monthly figure from a few days.

### Example

Oldest mail 2024-01-01, now 2024-03-01 → 60 days.
3 sent and 6 received mails:

| | per day | per week | per month |
|-|---------|----------|-----------|
| sent | 3 / 60 = 0.05 | 3 / 8.57 = 0.35 | 3 / 1.97 = 1.52 |
| received | 6 / 60 = 0.10 | 6 / 8.57 = 0.70 | 6 / 1.97 = 3.04 |

### Why from the oldest mail and not "only periods with mail"?

The history-wide average answers "how much mail do I deal with on a typical
day/week/month since I have this account". Quiet periods (holidays, a month
you didn't send anything) are part of that and pull the average down. Use
`--since`/`--until` to look at a specific period, e.g. only the last year.
