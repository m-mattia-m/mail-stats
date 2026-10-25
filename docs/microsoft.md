# Microsoft accounts: Hotmail, Outlook.com, Live, Microsoft 365

Microsoft no longer accepts passwords (not even app passwords) for IMAP on
personal accounts – `@hotmail.com`, `@outlook.com`, `@live.com`, `@msn.com` –
and most Microsoft 365 work/school tenants have switched it off too. IMAP
access needs an **OAuth2 access token** instead. mail-stats supports that
with `auth: oauth2`:

```
mail-stats auth login hotmail
  └─ asks Microsoft for a device code
  └─ you open https://microsoft.com/devicelogin, enter the code, sign in, allow access
  └─ mail-stats stores the refresh token in ~/.mail-stats/tokens/hotmail.json (0600)

mail-stats get stats --context hotmail
  └─ uses the stored refresh token to get a fresh access token (valid ~1 hour)
  └─ logs in to outlook.office365.com with IMAP AUTHENTICATE XOAUTH2
```

You do this once per account. After that, every `get stats` renews the token
silently; you only sign in again if the token is revoked or unused for a long
time (Microsoft's default is 90 days).

Setting it up has three parts: register an app with Microsoft (once, for all
your accounts), create the context, log in.

## Do I really need the Entra/Azure setup for a private Hotmail address?

Yes – also for a plain personal address like `@hotmail.ch`. The chain is:

1. Microsoft only accepts **OAuth2** for IMAP on personal accounts; passwords
   and app passwords are rejected.
2. Every app that asks Microsoft for an OAuth2 token must identify itself
   with a **client ID**. Client IDs only come from an **app registration**.
3. App registrations can only be created **inside a directory** (tenant). A
   personal Microsoft account has none, so you create one with a free Azure
   account (see [step 1](#do-you-have-a-directory)).

There is no shared mail-stats client ID, and `oauth2.tokenCommand` doesn't
avoid this either: external token tools (oama, `mutt_oauth2.py`, …) need a
client ID for exactly the same reason.

The good news: it's a **one-time** setup of about 10 minutes, and **one**
registration covers all your Microsoft addresses. Nothing in it is billed
(Azure may ask for a credit card only to verify your identity).

**Why not reuse another app's client ID?** Some guides tell you to "borrow"
the public client ID of an existing mail program (often Thunderbird's). That
works technically, but you then sign in *as that program*, which is against
Microsoft's terms and can be blocked by Microsoft or the app's vendor at any
time. mail-stats doesn't do this and it isn't recommended.

## 1. Register an app with Microsoft (one time)

OAuth2 needs a **client ID** – the ID of an "app registration" that says
"mail-stats wants to read mail via IMAP". There is no shared mail-stats
registration, so you create your own. It's free and takes about five minutes.

### Do you have a directory?

App registrations live in a Microsoft Entra **directory** (tenant).

- **Work/school account (Microsoft 365):** you already have one – but your
  admin may have to allow app registrations or do this part for you.
- **Personal account (Hotmail/Outlook.com) only:** signing in to the portal
  with it puts you in a tenant where you can't create anything ("the ability
  to create applications outside of a directory has been deprecated"). Create
  a free [Azure account](https://azure.microsoft.com/free/) with your
  Microsoft account first; that creates a directory for you. Azure may ask for
  a credit card to verify your identity, but app registrations themselves are
  free and nothing here uses paid services.

### Create the registration

1. Open the [Microsoft Entra admin center](https://entra.microsoft.com)
   (or the [Azure portal](https://portal.azure.com) → *Microsoft Entra ID*) and
   sign in.
2. Go to **Applications → App registrations → New registration**.
3. Fill in:
   - **Name:** `mail-stats` (only you will see it, on the consent screen).
   - **Supported account types:**
     - for Hotmail/Outlook.com: **Personal Microsoft accounts only**
     - for both personal and work accounts: **Accounts in any organizational
       directory and personal Microsoft accounts**
     - for work accounts of this tenant only: **Accounts in this
       organizational directory only**

     Choose carefully: changing this later can break the registration.
   - **Redirect URI:** leave empty.
4. Click **Register**. On the overview page, copy the
   **Application (client) ID** (a UUID like `1b2c3d4e-…`). That's your
   `clientId`. It is not a secret.
5. Open **Authentication** and set **Allow public client flows** to **Yes**
   (in some portal versions it's under *Advanced settings*, in newer ones on
   the *Settings* tab). **Save.** Without this, `mail-stats auth login` fails
   with `AADSTS70002: … must be marked as 'mobile'` (or `AADSTS7000218`).
   The change can take a minute to apply. If the error persists, also click
   **Add a platform → Mobile and desktop applications**, tick
   `https://login.microsoftonline.com/common/oauth2/nativeclient` and save.
6. Optional but tidy: **API permissions → Add a permission → Microsoft Graph →
   Delegated permissions**, add **IMAP.AccessAsUser.All** and
   **offline_access**. mail-stats requests these scopes anyway, so personal
   accounts work without this step; for work tenants that require admin
   consent, the admin can grant it here (**Grant admin consent**).

You don't need a client secret or certificate – mail-stats is a "public
client", like a desktop mail app.

## 2. Create the context

```bash
mail-stats config set-context hotmail \
  --host outlook.office365.com \
  --username me@hotmail.com \
  --auth oauth2 \
  --oauth2-client-id 1b2c3d4e-0000-0000-0000-000000000000
```

| Account | `--oauth2-tenant` |
|---------|-------------------|
| Hotmail, Outlook.com, Live, MSN (personal) | `consumers` (default, can be omitted) |
| Microsoft 365 work/school | `organizations`, or your tenant ID / domain (e.g. `contoso.onmicrosoft.com`) |
| both kinds with one registration | `common` |

The host is `outlook.office365.com` (port 993, TLS) for both personal and
Microsoft 365 accounts. In the config file:

```yaml
- name: hotmail
  host: outlook.office365.com
  username: me@hotmail.com
  auth: oauth2
  oauth2:
    clientId: 1b2c3d4e-0000-0000-0000-000000000000
    tenant: consumers          # default
    # scopes:                  # default:
    #   - https://outlook.office.com/IMAP.AccessAsUser.All
    #   - offline_access
```

`passwordCommand`, `passwordEnv`, `password` and `MAIL_STATS_PASSWORD` are
ignored for OAuth2 contexts.

## 3. Log in

```bash
mail-stats auth login hotmail
```

```
To sign in as me@hotmail.com, open https://microsoft.com/devicelogin in a browser and enter the code ABCD-EFGH
Waiting for the sign-in to complete...
Logged in to context "hotmail". Token stored in /Users/me/.mail-stats/tokens/hotmail.json.
```

1. Open the URL in any browser (also on your phone – the device-code flow
   doesn't need a browser on the same machine).
2. Enter the code.
3. Sign in with **the mail account** (`me@hotmail.com`), not with the account
   you used to create the app registration, if that's a different one.
4. Microsoft shows a consent screen for your `mail-stats` app ("unverified"
   is expected – it's your own registration) asking to access your mail via
   IMAP and to keep access. Accept.

Then:

```bash
mail-stats auth status
# NAME      USERNAME         TOKENS FROM   STATUS
# hotmail   me@hotmail.com   auth login    logged in

mail-stats get folders --context hotmail
mail-stats get stats --context hotmail
```

## Several Microsoft accounts

One app registration (with a fitting *supported account types*) works for all
your accounts. Create one context per account and log in to each:

```bash
mail-stats config set-context hotmail --host outlook.office365.com --username me@hotmail.com \
  --auth oauth2 --oauth2-client-id 1b2c3d4e-…
mail-stats config set-context outlook --host outlook.office365.com --username other@outlook.com \
  --auth oauth2 --oauth2-client-id 1b2c3d4e-…

mail-stats auth login hotmail
mail-stats auth login outlook      # sign in with other@outlook.com this time
mail-stats get stats --contexts hotmail,outlook
```

Tip: if the browser automatically signs you in with the previous account,
open the device-login URL in a private window.

## Logging out / revoking

```bash
mail-stats auth logout hotmail     # deletes ~/.mail-stats/tokens/hotmail.json
```

To also revoke the access on Microsoft's side, remove the app on
<https://account.live.com/consent/Manage> (personal accounts) or
<https://myapps.microsoft.com> (work accounts).

## Where tokens are stored

`tokens/<context>.json` in the same directory as the config file
(`~/.mail-stats/tokens/` by default; with `--config /x/config.yaml` it's
`/x/tokens/`). Files are created with mode `0600`, the directory with `0700`.
The file holds the refresh token, which gives read access to your mailbox
until it expires or is revoked – treat it like a password.

Because a refresh renews and rewrites the file, the tokens directory must be
**writable** when you run `get stats` (relevant for the
[container image](container.md#microsoft--oauth2-accounts)).

## Alternative: an external token command

If you already use a tool that manages OAuth2 tokens (for example
[oama](https://github.com/pdobsan/oama), or `mutt_oauth2.py`), let mail-stats
call it instead of using the built-in login:

```bash
mail-stats config set-context hotmail --host outlook.office365.com --username me@hotmail.com \
  --auth oauth2 --oauth2-token-command 'oama access me@hotmail.com'
```

The command runs with `sh -c` and must print a valid **access token** on
stdout. `auth login` is then not used (`auth status` shows `tokenCommand`).

## Folders on Outlook.com

Typical folders and how mail-stats counts them:

| Folder | Role |
|--------|------|
| `Inbox`, `Junk`, `Archive`, `Deleted`, your own folders | received |
| `Sent` | sent |
| `Drafts` | excluded |
| `Outbox`, `Notes` (if present) | received – exclude them with `--exclude-folder Outbox --exclude-folder Notes` if they contain anything |

Check with `mail-stats get folders --context hotmail`.

## Troubleshooting

| Error | Cause / fix |
|-------|-------------|
| `context "hotmail" is not logged in: run mail-stats auth login hotmail` | No token stored yet (or you logged out). |
| `auth oauth2 needs oauth2.clientId … or oauth2.tokenCommand` | Add `--oauth2-client-id`. |
| `start device login: … AADSTS700016` / `unauthorized_client` | Client ID wrong, or the registration's *supported account types* don't include this kind of account (e.g. "organizational only" with a Hotmail address). |
| `start device login: … AADSTS70002: The provided client is not supported for this feature. The client application must be marked as 'mobile.'` or `device login: … AADSTS7000218` | **Allow public client flows** is not enabled (step 1.5): *Authentication → Allow public client flows → Yes → Save*, wait a minute, retry. If it persists, also add the platform *Mobile and desktop applications* with the redirect URI `https://login.microsoftonline.com/common/oauth2/nativeclient`. |
| `device login: … authorization_declined` | You clicked *Cancel* on the consent screen – run `auth login` again. |
| `device login: … expired_token` | The code wasn't entered within 15 minutes – run `auth login` again. |
| `AADSTS65001` / consent required | A work tenant requires admin consent: ask your admin to *Grant admin consent* for the registration. |
| `login as … with OAuth2: AUTHENTICATE failed` | Token was issued for a different account than `username` (you signed in with the wrong account), IMAP is disabled for the mailbox (Outlook.com: check the POP/IMAP option in the mail settings under *Sync email*; Microsoft 365: an admin enables IMAP for the mailbox), or the scope is wrong. Run `auth logout` and `auth login` again with the right account. |
| `refreshing the access token failed, run mail-stats auth login … again` | The refresh token expired or was revoked (password change, removed consent, 90 days unused). Log in again. |
