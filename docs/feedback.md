# Feedback

Every app's app switch ends with "Report a problem or idea". It opens a small form over the page: a choice between "Something's wrong" and "I'd like…", a one-line summary, and details, with a note saying the report goes to the people who build Heliosian along with the page's address, the reporter's email, and their browser details. Sending closes the form and says "Thanks, we got it." The form, its button, and the toast live in `web/common/toolbar.js` and `web/common/toolbar.css`, so every app has them with no markup of its own, and `toolbar.js` keeps the last few JavaScript errors and unhandled promise rejections the page raised (message and file, five at most) to send with the report.

A report goes to a spreadsheet, not to GitHub. The super admins hear about it by mail and work through the queue on Heliosian's own admin page, and only what one of them edits and keeps becomes an issue in the open. Nothing a person reports reaches GitHub on its own.

## The endpoint

`POST /api/feedback` is registered on every app's mux (`internal/feedback`, wired in `internal/app.NewCore` beside the app switch's route). The body is JSON: `kind` (`bug` or `idea`), `summary` (120 characters at most), `details` (4000), and the page's context - `url`, `page` (the document title), `viewport`, `screen`, `language`, `timezone`, and `errors`. Everything that says who and where comes from the server, never the client: the reporter's email from the session, the app from the mux the route is on, whether they are a super admin from the Config sheet, the browser from the request's user agent, and the time from the server's clock. Every client-supplied field is trimmed and clipped.

The handler validates, hands the report to a queue, and answers `202 Accepted` at once; one goroutine drains the queue, writes each report to the Reports tab and then mails the super admins, neither of which anyone waits on. A save that fails is logged at error level with the whole report, so nothing is lost. A person may send five reports in ten minutes; a sixth gets a 429 asking them to wait. A queue that is full (64 reports waiting) answers 503.

## The Reports tab

One spreadsheet, `FEEDBACK_SHEET`, one tab, `Reports`, one row per report, appended as it arrives and never deleted. Its columns are in `feedback.ReportColumns`, which `cmd/createtabs` writes: the `ID` that names the row, `Received`, the `App` key and `App Name`, `Kind`, `Status`, the reporter's `Summary` and `Details`, their `Email` and `Role`, the page's `URL`, `Page`, `Browser`, `Viewport`, `Screen`, `Language` and `Time Zone`, the `Errors` the page had raised, and - once an admin has dealt with it - the `Issue` it became, when it was `Handled` and by whom.

`Status` is the whole of the workflow: `New` until an admin acts, then `Filed` against an issue or `Dismissed` without one. The tab loads at startup, refreshes on the same five minutes as every other model, and is written through the shared sheet queue, so the page shows a filing the instant it happens rather than on the next refresh.

## Telling the super admins

Every saved report is mailed to the super admins the Config sheet names - the same list that may read the queue, so whoever can triage is whoever hears. It goes out through the portal's own sender and address (`MAIL_FROM`, HCA-Team), and carries the kind, the app, the summary, the details, who reported it, and a link straight to the report on the admin page. Without a mail sender, or with nobody on the list, the report is still saved and simply not announced.

## The queue

Heliosian's admin page (`/admin` on the front page's host) has a Feedback panel: every report newest first, filtered to New, Filed, Dismissed or all, with a count of what is waiting beside the tab. The reports carry who reported them and what page they were on, so the panel is the super admins' alone - an app admin does not see the tab at all, and `/api/admin/feedback` answers them 403.

Opening one shows what the person wrote and everything the row holds, including their address, and below it the issue as it would be filed, ready to edit: a title, a body, and the labels. That draft is `feedback.Strip`, which is where the personal detail is taken out:

- The Reporter section the old triage issues carried is gone; nothing names who reported it.
- Every email address anywhere in the summary, the details or the errors becomes `[email removed]`, including one the reporter typed themselves.
- The page's address keeps its path and loses its query string and fragment, which may carry a token or somebody's address.
- `Role` stays out of the issue.

What stays is the reporter's own words and the context that makes a bug reproducible: the app, the page title, the path, the browser, the viewport and screen, the language and time zone, when it was reported, and the errors. Those are the substance of a bug report rather than a name, and an admin who wants any of them gone edits them out - whatever is in the boxes when they press the button is exactly what GitHub gets. As a last guard the server refuses a title or body that still has an address in it and says which one.

Filing writes the issue's address and the admin's own back to the row and moves it to Filed; a report already filed cannot be filed twice. Dismissing marks it Dismissed with no issue. Neither deletes anything: the row, with everything it arrived with, stays in the tab.

## The issue

A kept report becomes an issue in `heliosian/heliosian` itself - the primary repository, in the open, which is the reason nothing personal may reach it. The title starts as the app's name in brackets and the summary, `[Helios When] Next month shows nothing`, and the labels start as the kind (`bug` or `idea`) and `app:<key>` (`app:home`, `app:who`, `app:team`, `app:birthday`, `app:celebrate`, `app:calendar`, `app:loop`, `app:ask`); each label has to exist on the repository ahead of time, and `gh label list --repo heliosian/heliosian` says which do.

It is filed by a GitHub App, not by a person, so the issue's author is the app's own bot account and an issue built out of somebody's report is plainly the robot's doing rather than the admin's own words. `internal/feedback/github.go` signs a short-lived RS256 JWT with the app's key, asks `GET /repos/heliosian/heliosian/installation` which installation it is (so nothing has to carry an installation id), trades the assertion for an installation token, and keeps that token until five minutes before its hour is out.

## Setup

The GitHub App lives on the heliosian organization, under Settings › Developer settings › GitHub Apps. It needs repository permission Issues set to read and write, nothing else, no webhook, and it has to be installed on `heliosian/heliosian` alone. Two things come out of creating it: its App ID, on the app's own settings page, and a private key, which GitHub generates once and hands over as a PEM download - GitHub keeps no copy, so a lost key is replaced rather than recovered. Both live in Secret Manager, `heliosian-github-app-id` and `heliosian-github-app-key`, each granted to `directory@` on the secret itself - creating a secret grants nothing, and a deploy naming one the runtime identity cannot read is refused as `Permission denied on secret`, which is also what a secret that does not exist at all looks like:

    printf %s <app id> | gcloud secrets create heliosian-github-app-id --project heliosian --replication-policy automatic --data-file=-
    gcloud secrets create heliosian-github-app-key --project heliosian --replication-policy automatic --data-file=<the downloaded pem>
    gcloud secrets add-iam-policy-binding heliosian-github-app-id --project heliosian --member serviceAccount:directory@heliosian.iam.gserviceaccount.com --role roles/secretmanager.secretAccessor
    gcloud secrets add-iam-policy-binding heliosian-github-app-key --project heliosian --member serviceAccount:directory@heliosian.iam.gserviceaccount.com --role roles/secretmanager.secretAccessor

They reach the service through `cmd/deploy`, which names them in `secrets`; a plain push deploys only the image, so both have to be in place, through a `cmd/deploy` run, before a build that carries this is deployed. Locally, real-data mode reads `local/creds/github-app.id` and `local/creds/github-app.pem`. Both are required: the server refuses to start without them.

The key has no expiry, so nothing has to be renewed on a schedule, which is the point of an app over the yearly token it replaced. Where the app is installed and what it may do is the app's own page on GitHub and nowhere else; that it still works is the first filing after a change, which either opens an issue or answers with GitHub's own refusal, logged in full.

## Reports carried over from triage

Reports whose `ID` reads `triage-<number>` came from `heliosian/triage`, the private repository every report was filed to before the Reports tab existed. They carry what their old rendered issue still said, read back apart into the columns it came from: an issue open there is New here, a closed one Dismissed, and their `Issue` points back at the triage issue itself rather than at anything in the primary repository. The two of them filed on GitHub by hand rather than through the toolbar name no app and carry no reporter or browser context, since their bodies never held any.

Nothing writes to `heliosian/triage` and nothing reads it. The tab holds everything it did, so the repository, the fine-grained personal access token in `local/creds/github.token` and the `heliosian-github-token` secret are all retired.
