# Helios MCP

Helios MCP, at `mcp.heliosian.com`, serves the data model (`docs/datamodel.md`) to Claude and any other client that speaks the Model Context Protocol, read as the person who connected it. The package is `internal/mcp`. It is an app in `model.Apps` like any other, so its place in the app switch follows its row on the Apps sheet's `Visibility` tab. That row only decides who is shown the app: anyone the directory admits can sign in on any host and use the query API, which is everything the tools read, so the row is no gate on the data.

## Connecting

The host's front page (`web/mcp/index.html`) gives the address to add, the host itself, as a custom connector in Claude or ChatGPT or with `claude mcp add --transport http` in Claude Code. The client finds everything else itself.

The MCP endpoint answers at both `POST /` and `/mcp`, since people type the bare host and some clients expect the conventional path: the Go SDK's streamable HTTP handler, stateless, answering in JSON. A `GET /` is the front page. Each request carries an `Authorization: Bearer` token and is refused with a 401 without a good one. The 401's `WWW-Authenticate` names the protected-resource metadata for the path asked, which is how a client learns where to sign in.

## Signing in

The host is its own OAuth 2.1 authorization server, built on the apps' Google sign-in rather than beside it. It keeps nothing: every client id, code and token is a JSON payload signed with an HMAC under a key derived from `SESSION_KEY` (`MCPKey` in `internal/app/wiring.go`), separate from the session cookie's key, so a token can never be used as a cookie or a cookie as a token.

- `/.well-known/oauth-protected-resource` names the root as the resource, and the same under `/mcp` names `/mcp`, each with the host as its authorization server. `/.well-known/oauth-authorization-server` names the endpoints below. Any other `/.well-known/` path is a 404, OpenID Connect's discovery document among them: the host issues no ID tokens, and clients fall back to the OAuth metadata.
- `POST /oauth/register` is dynamic client registration. It accepts redirect addresses that are https, or plain http on a loopback address, and answers a client id that holds the client's name, its redirect addresses and a random value, so two registrations are two clients. Clients are public: there is no secret, only PKCE.
- `GET /oauth/authorize` sits behind the normal sign-in, so someone already signed in to any app goes straight to it and anyone else signs in with Google first. The page (`web/mcp/authorize.html`) shows the client's name, the account it will read as and where approving sends the browser, from `POST /api/mcp/request`. Allow posts to `/api/mcp/approve`, Deny to `/api/mcp/deny`, and each answers the client's redirect address with a code or `access_denied` and the request's `state`. All three check the authorization request again: the client id is one this server signed, the redirect address is one the client registered, the response type is `code` and there is an S256 code challenge. They also refuse anything the browser does not mark `Sec-Fetch-Site: same-origin`. Approving binds the code to the signed-in person, never a Spoof Mode view, and is refused for anyone the data model does not sign in.
- `POST /oauth/token` exchanges a code for an access token. The code must still be within `codeLength`, be presented by the client it was issued to with the same redirect address, and come with the verifier that hashes to its challenge. Because nothing is stored, a code is not single-use; its short life and PKCE are what bind it. There are no refresh tokens: a token lasts `tokenLength`, and the client signs in again after it.

The endpoint refuses a token once it has expired, once the person signs out of any app after it was issued (the same `Signed Out` record that ends their sessions, `docs/dev.md`), and once the directory or the data model stops admitting them. Those paths, and the OAuth paths a client reaches without a cookie, are named in `auth.Public` (`docs/dev.md`, Hosts and files).

## Tools

Every tool is read-only and runs as the person the token names, resolved for each call through the data model's own sign-in (`Model.SignedIn`). Every read goes through `Model.Run`, so the read policies and the consent step decide what each tool answers, exactly as they do for `/api/q`. The server's instructions, sent when a client connects, carry the query language from `db.Language`, the same text the Admin Query page's composer gives Claude. An answer longer than `maxOutput` is refused with a word on narrowing it.

A client decides whether to use a connector mostly from its tools' names and descriptions, and many never show the model the server's instructions. So every tool's name starts `helios_`, and every description says it is Helios School's, the ones a question starts from (`helios_search`, `helios_whoami`, `helios_events`) saying what questions they answer; `TestEveryToolSaysItIsHelios` holds every tool to that. The server names itself Helios School and describes the community in `about`, which the instructions open with.

| Tool | What it answers |
| --- | --- |
| `helios_search` | the Admin search's word and meaning results (`Searcher.Words` and `Meaning`), each with its table and name |
| `helios_whoami` | the person's row, addresses and memberships, the groups they manage (`manages`) and the apps they are an admin of (`admin_of`) |
| `helios_events` | events starting in a range of days, two weeks from today by default |
| `helios_find_people` | people by search words, role group, grade and classroom name |
| `helios_read_document` | a document's tree of parts, links, images and extracts, and the Markdown of each extract, read from the bucket through `Model.BlobCell` |
| `helios_get` | one row with the names of what it references, and the first few rows of each table and column that point at it |
| `helios_group` | a group with its managers (the effective members of each `managed_by` up its parents), its effective members and their reasons, its rules and the groups under it |
| `helios_query` | a query in the language, run as `/api/q` runs it: the canonical form, the count, the rows with their filled columns and the rows the includes brought |
| `helios_describe_schema` | every table with its description and column names, from `db.Tables` |
| `helios_describe_table` | one table's columns in full, from `db.DescribeTable` (the composer's description, private columns left out), and the columns elsewhere that point at it |
| `helios_policies` | `db.PolicySource`: the definitions a query may call and every clause |

## Links

Every record a tool answers that has a page on the Helios apps carries `href`, the address of that page, and the instructions and `helios_search`'s description tell the model to link a record to it whenever it names one and never to make an address up. The field is `href` because `link` and `url` are columns already (`DOCUMENT.link`, and `url` on groups and documents). The rule is `db.Link` in `internal/db/link.go`, beside the data model, so anything else that needs a record's page reads the same one: each kind's page on its app, by the record's `slug` or else its ID, under the app's host on the server's domain (`Deps.Domain`, the production domain or the dev server's with its port). A wiki page links to its page on Helios Wiki, and any other document to its own address on the web. A record of any other kind has no `href`; nothing links to Helios Admin, which is an admin's tool.

Each call logs an `mcp: tool` line with the tool, the viewer's person ID, how long it took and any error.

## Tests

`internal/mcp/mcp_test.go` loads the sample data model, registers the real routes behind `auth.Fixed`, and goes through what a client does: registering, approving, exchanging the code, then connecting with the SDK's own client and calling each tool, at `/mcp` and at the root. It also covers the refusals, at both: no token or a forged one, a wrong verifier, another client's exchange, an unregistered redirect, a cross-site approval, and a token from before a sign-out.
