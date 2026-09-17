# Feedback

Every app's app switch ends with "Report a problem or idea". It opens a small form over the page: a choice between "Something's wrong" and "I'd like…", a one-line summary, and details, with a note saying the report goes to the people who build Heliosian along with the page's address, the reporter's email, and their browser details. Sending closes the form and says "Thanks, we got it." The form, its button, and the toast live in `web/common/toolbar.js` and `web/common/toolbar.css`, so every app has them with no markup of its own, and `toolbar.js` keeps the last few JavaScript errors and unhandled promise rejections the page raised (message and file, five at most) to send with the report.

## The endpoint

`POST /api/feedback` is registered on every app's mux (`internal/feedback`, wired in `internal/app.NewCore` beside the app switch's route). The body is JSON: `kind` (`bug` or `idea`), `summary` (120 characters at most), `details` (4000), and the page's context - `url`, `page` (the document title), `viewport`, `screen`, `language`, `timezone`, and `errors`. Everything that says who and where comes from the server, never the client: the reporter's email from the session, the app from the mux the route is on, whether they are a super admin from the Config sheet, the browser from the request's user agent, and the time from the server's clock. Every client-supplied field is trimmed and clipped.

The handler validates, hands the report to a queue, and answers `202 Accepted` at once; one goroutine drains the queue and files each report with GitHub, which takes a few hundred milliseconds nobody waits on. A filing that fails is logged at error level with the whole rendered issue, so nothing is lost. A person may send five reports in ten minutes; a sixth gets a 429 asking them to wait. A queue that is full (64 reports waiting) answers 503.

## The issue

Reports become issues in the private repository `github.com/heliosian/triage`, filed through the REST API with a fine-grained personal access token whose only access is that repository's issues, read and write. The token reaches the server as `GITHUB_TOKEN`, or locally as `creds/github.token`, and is required: the production assembly refuses to start without it. In sample mode the issue is printed to the console instead, exactly as it would reach GitHub.

The title is the app's name in brackets and the summary: `[Helios Calendar] Next month shows nothing`. The body opens with the details in the reporter's own words, then a Context table (app, page, address, role, browser, viewport, screen, language, time zone, and the time in school time), then the recent errors in a collapsed block when there are any, and last a Reporter section holding the email, headed by a note to remove it before filing anything publicly. The labels are the kind (`bug` or `idea`), `app:<key>` (`app:home`, `app:who`, `app:team`, `app:birthday`, `app:celebrate`, `app:calendar`), and `untriaged`; each has to exist on the triage repository ahead of time, and `gh label list --repo heliosian/triage` says which do.

## Triage

Issues wait in `heliosian/triage` under `untriaged`. One worth keeping gets a fresh, clean issue in the public repository, written without the reporter's details, and the triage issue is closed with a link to it. GitHub's transfer is not the way: it carries the body, the Reporter section, and every comment across as they are.

## Setup

The token has to be a fine-grained personal access token on the heliosian organization: repository access limited to `heliosian/triage`, repository permission Issues set to read and write, nothing else, with the longest expiry GitHub allows (a year). Its scope and expiry are written down only on GitHub's fine-grained tokens page (Settings › Developer settings); whether the one on disk still works, and until when, is one request:

    curl --silent --include --header "Authorization: Bearer $(cat creds/github.token)" https://api.github.com/repos/heliosian/triage

a `200` whose `github-authentication-token-expiration` header names the day. It has to live in Secret Manager as `heliosian-github-token`, granted to `directory@` like the other secrets (`docs/deploy.md`, IAM, says how to read both), and reaches the service through `cmd/deploy`, which names it in `secrets`; a plain push deploys only the image, so the secret has to be in place, through a `cmd/deploy` run, before a build that carries the endpoint is deployed. Locally, real-data mode reads `creds/github.token`. When the token expires, GitHub answers 401, every filing fails and is logged in full, and a new token in the secret's next version puts it right.
