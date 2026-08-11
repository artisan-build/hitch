# The hitch claim contract (v1)

This is the wire contract a server implements so that `hitch install <url> --claim <code>
--claim-url <your-endpoint>` can exchange a **single-use claim code** for a durable bearer token.
It is one route: read a JSON body, look up a code, return a token or an error enum. No hitch source
access is needed to implement it, and any language that can serve HTTPS and JSON qualifies.

Why it exists: a token pasted into a one-liner lands in shell history and npm's debug logs — copies
no tool can later find and revoke. A claim code may land there instead, because a code is worthless
once the token it bought has been used.

## Request

hitch sends one POST to the claim URL:

```http
POST <claim-url>
Content-Type: application/json
Accept: application/json
User-Agent: hitch/<version>

{"claim_code": "A1B2-C3D4", "version": 1}
```

- The code travels **in the body, never in the URL** — query strings land in server access logs.
- `version` is an integer in the body. It is the contract version the client speaks, currently `1`.

## Success — `200 OK`

```json
{
  "version": 1,
  "token": "sk_live_…",
  "name": "ballast",
  "expires_at": "2027-01-01T00:00:00Z"
}
```

| field | required | meaning |
|---|---|---|
| `version` | yes | the contract version you speak (`1`) |
| `token` | yes | the durable credential hitch installs |
| `name` | no | a suggested server name; the user's explicit `--name` always overrides it |
| `expires_at` | no | RFC 3339 or `null`; advisory — hitch surfaces it and does nothing else |

The client ignores unknown fields, so the response can grow without a version bump.

**There is no `server_url` field, deliberately.** hitch installs the URL the user passed; a response
cannot redirect the install to a different host. If a client receives such a field it must ignore it.

## Errors

Errors are `4xx`/`5xx` with a JSON body:

```json
{"version": 1, "error": "code_already_claimed", "message": "This code was already used on 2026-08-10."}
```

`error` is a **stable machine-readable enum** — it is what the client branches on. `message` is
human prose shown to the user verbatim; you control that sentence. Because it is printed verbatim,
**never put a token, a claim code, or any other secret in `message`** — the client cannot tell a
credential from prose, and whatever you write there lands in the user's terminal.

| `error` | suggested HTTP | meaning |
|---|---|---|
| `invalid_code` | 400 | malformed code — a user typo |
| `code_not_found` | 404 | no such code |
| `code_already_claimed` | 409 | the code's token was already **used** (see below — redemption alone does not burn) |
| `code_expired` | 410 | the code's TTL passed; the user needs a new one-liner |
| `unsupported_version` | 400 | you do not speak the client's contract version |
| `server_error` | 5xx | transient; safe for the user to retry |

Clients branch on the enum, never on the HTTP status, and treat an unknown enum value as a generic
failure. The statuses above are guidance; the enum is the contract.

Because `code_already_claimed` means *claimed and used*, write its `message` accordingly: "This code
was already used to set up a working connection" is true; "this code was already redeemed" is not.

## Make-before-break — when a code burns

> **The claim code survives until the token it was swapped for is successfully used for the first
> time — or until the code's own TTL expires, whichever comes first. Redemption alone does not burn
> the code; only demonstrated success does.**

You can detect this on your own infrastructure with no extra machinery: the server issuing the token
is the server authenticating it, so the first authenticated request bearing the token flips the code
to burnt.

Consequences you must implement:

- **A re-claim before first use returns a usable token.** Simplest conforming behaviour: return the
  same token. Also conforming: mint a fresh token and invalidate the pending one. Not conforming: an
  error before first use, or two live tokens outstanding.
- **At most one live token per code**, ever.
- After the token's first successful use, further claims return `code_already_claimed`.

This is what makes a lost response harmless: the user reruns the same one-liner and it works. The
code's exposure window is bounded by its TTL — keep the TTL short; it is the only knob.

## Transport

- **HTTPS is required.** Clients refuse a plaintext claim URL before any request is made.
- Redirects are followed to a maximum of 5, same-origin only.
- Clients use a 30-second timeout and make **one attempt with no automatic retry** — under
  make-before-break a failed exchange leaves the code live, so the user simply reruns the command.

## Versioning

If the client's `version` is one you do not speak, return the `unsupported_version` error. If your
response carries a `version` greater than the client's, the client refuses it with a
"newer contract" message — so only raise your response `version` when you actually break the shape
described here. Additive fields need no version change; clients ignore what they do not know.
