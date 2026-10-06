# Feedback

Every app's app switch ends with "Report a problem or idea". It opens a small form over the page: a choice between "Something's wrong" and "I'd like…", a one-line summary, details, and an optional screenshot, with a note saying the report goes to the people who build Heliosian along with the page's address, the reporter's email, their browser details, and any screenshot they add. The screenshot is one the person takes themselves: "Add a screenshot" opens the file picker (the photo library on a phone), and pasting an image anywhere in the form takes it too; a thumbnail shows what will be sent, with Remove beside it. The page never captures itself - redrawing the page into an image mis-draws scrolled and layered layouts, so it would show something other than what the person saw. Sending closes the form and says "Thanks, we got it." The form lives in `web/common/feedback.js`, its button in the app switch (`web/common/appswitch.js`), both styled by `web/common/toolbar.css`, so every app has them with no markup of its own; the thanks goes through the shared `toast` into the `#toast` every app's page carries; and `feedback.js` keeps the last few errors the page met (message and file, five at most) to send with the report: JavaScript errors and unhandled promise rejections, and the top bar's own requests that failed, which `noteError` records (`docs/toolbar.md`).

A report goes to a spreadsheet, not to GitHub. The super admins hear about it by mail and work through the queue on Heliosian's own admin page, and only what one of them edits and keeps becomes an issue in the open. Nothing a person reports reaches GitHub on its own.

## The endpoint

`POST /api/feedback` is registered on every app's mux (`RegisterFeedback` in `internal/model/feedback_intake.go`, wired in `internal/app.NewCore` beside the resource API's routes). The body is a multipart form: a `report` field holding JSON - `kind` (`bug` or `idea`), `summary` (120 characters at most), `details` (4000), and the page's context, `url`, `page` (the document title), `viewport`, `screen`, `language`, `timezone`, and `errors` - and an optional `screenshot` file, `screenshotLimit` bytes at most, which has to sniff as PNG, JPEG, GIF or WebP. Everything that says who and where comes from the server, never the client: the reporter's email from the session, the app from the mux the route is on, whether they are a super admin from the Config sheet, the browser from the request's user agent, and the time from the server's clock. Every client-supplied field is trimmed and clipped.

The handler validates, commits the report as a new row of the Reports tab, made as the reporter, and answers `204 No Content`; the commit takes the report into memory at once and writes the row behind the request (`docs/storage.md`). The screenshot is written to the media bucket after the answer, under `feedback/` and named by its content's hash, and the mail to the super admins goes out once it is stored; the reporter waits on neither. The public media routes serve only their own folders (`blob.Register`), so nothing under `feedback/` is reachable except through the screenshot route below. A commit that is refused answers 503 and logs the whole report at error level, so nothing is lost. A person may send `feedbackPerWindow` reports in each `feedbackWindow` (both in `internal/model/feedback_intake.go`, counted by `internal/ratelimit`); one more gets a 429 asking them to wait.

## The Reports tab

One spreadsheet, `FEEDBACK_SHEET`, one tab, `Reports`, one row per report, appended as it arrives and never deleted. Its columns are in `model.ReportColumns`, which `tools/createtabs` writes: the `ID` that names the row, `Received`, the `App` key and `App Name`, `Kind`, `Status`, the reporter's `Summary` and `Details`, their `Email` and `Role`, the page's `URL`, `Page`, `Browser`, `Viewport`, `Screen`, `Language` and `Time Zone`, the `Errors` the page had raised, once an admin has dealt with it the `Issue` it became, when it was `Handled` and by whom, and the `Screenshot`'s object name in the media bucket when one was sent. A sheet laid out before a column existed gets it from a `tools/createtabs` run, which the server's load needs before it will start.

`Status` is the whole of the workflow: `New` until an admin acts, then `Filed` against an issue or `Dismissed` without one. The Feedback sheet is a part of the one store like every app's sheet (`docs/storage.md`): it loads at startup and refreshes with the rest, every change is a commit, and its Change Log holds what a changed cell held before, so the page shows a filing the instant it happens and the status it replaced is on record.

## Telling the super admins

Every saved report is mailed to the super admins the Config sheet names - the same list that may read the queue, so whoever can triage is whoever hears. It goes out through the portal's own sender and address (`mailFrom` in `internal/app/environment.go`), and carries the kind, the app, the summary, the details, who reported it, their browser (the user agent, which names the browser and the operating system), their window and screen size, the screenshot as an attachment when they sent one, and a link straight to the report on the admin page. The mail goes to the super admins alone, the same people the admin page shows the screenshot to. Without a mail sender, or with nobody on the list, the report is still saved and simply not announced.

## The queue

Heliosian's admin page (`/admin` on the front page's host) has a Feedback panel: every report newest first, filtered to New, Filed, Dismissed or all, with a count of what is waiting beside the tab. The reports carry who reported them and what page they were on, so the panel is the super admins' alone - an app admin does not see the tab at all. The panel reads the reports from the resource API as `feedback-reports` (`docs/api.md`, Feedback reports), which lists and answers them only to someone holding the `feedback` allowance, which the super admins hold, and the page shows the tab by that allowance in `/api/me`.

Opening one shows what the person wrote and everything the row holds, including their address and their screenshot (the bytes served by `GET /api/admin/feedback/{id}/screenshot` on Heliosian's host, to super admins alone), and below it the issue as it would be filed, ready to edit: a title, a body, the type and the labels. That draft is `model.StripReport`, which is where the personal detail is taken out:

- Nothing names who reported it.
- Every email address anywhere in the summary, the details or the errors becomes `[email removed]`, including one the reporter typed themselves.
- The page's address keeps its path and loses its query string and fragment, which may carry a token or somebody's address.
- `Role` stays out of the issue.
- The screenshot stays out of the issue: it can show anybody's name, family or event, and nothing in it can be redacted the way text is.

What stays is the reporter's own words and the context that makes a bug reproducible: the app, the page title, the path, the browser, the viewport and screen, the language and time zone, when it was reported, and the errors. Those are the substance of a bug report rather than a name, and an admin who wants any of them gone edits them out - whatever is in the boxes when they press the button is exactly what GitHub gets. As a last guard the server refuses a title or body that still has an address in it and says which one.

Filing (the report's `file` action, with the draft as edited) opens the issue, then writes the issue's address and the admin's own back to the row and moves it to Filed; a report already filed cannot be filed twice, and `can.file` is false on a server with no GitHub App, where the page offers only Dismiss. Dismissing (`dismiss`) marks it Dismissed with no issue. Neither deletes anything: the row, with everything it arrived with, stays in the tab.

## The issue

A kept report becomes an issue in `heliosian/heliosian` itself - the primary repository, in the open, which is the reason nothing personal may reach it. The title starts as the app's name in brackets and the summary, `[Helios When] Next month shows nothing`. The type starts from the kind, a `bug` filed as Bug and an `idea` as Feature; the types are the organization's, and `gh api orgs/heliosian/issue-types` lists them. The labels start as `app:<key>` for the app it was reported from; each label has to exist on the repository ahead of time, and `gh label list --repo heliosian/heliosian` says which do.

It is filed by a GitHub App, not by a person, so the issue's author is the app's own bot account and an issue built out of somebody's report is plainly the robot's doing rather than the admin's own words. `internal/feedback/github.go` signs a short-lived RS256 JWT with the app's key, asks `GET /repos/heliosian/heliosian/installation` which installation it is (so nothing has to carry an installation id), trades the assertion for an installation token, and keeps that token until five minutes before its hour is out.

## Setup

The GitHub App lives on the heliosian organization, under Settings › Developer settings › GitHub Apps. It needs repository permission Issues set to read and write, nothing else, no webhook, and it has to be installed on `heliosian/heliosian` alone. Two things come out of creating it: its App ID, on the app's own settings page, and a private key, which GitHub generates once and hands over as a PEM download - GitHub keeps no copy, so a lost key is replaced rather than recovered. Both live in Secret Manager, `heliosian-github-app-id` and `heliosian-github-app-key`, each granted to `directory@` on the secret itself - creating a secret grants nothing, and a deploy naming one the runtime identity cannot read is refused as `Permission denied on secret`, which is also what a secret that does not exist at all looks like:

    printf %s <app id> | gcloud secrets create heliosian-github-app-id --project heliosian --replication-policy automatic --data-file=-
    gcloud secrets create heliosian-github-app-key --project heliosian --replication-policy automatic --data-file=<the downloaded pem>
    gcloud secrets add-iam-policy-binding heliosian-github-app-id --project heliosian --member serviceAccount:directory@heliosian.iam.gserviceaccount.com --role roles/secretmanager.secretAccessor
    gcloud secrets add-iam-policy-binding heliosian-github-app-key --project heliosian --member serviceAccount:directory@heliosian.iam.gserviceaccount.com --role roles/secretmanager.secretAccessor

They reach the service through `tools/deploy`, which names them from `Secrets` in `internal/env`; a plain push deploys only the image, so both have to be in place, through a `tools/deploy` run, before a build that carries this is deployed. Locally, real-data mode reads them from `GITHUB_APP_ID` and `GITHUB_APP_KEY`, which `tools/devenv` exports. Both are required: the server refuses to start without them.

The key has no expiry, so nothing has to be renewed on a schedule. Where the app is installed and what it may do is the app's own page on GitHub and nowhere else; that it still works is the first filing after a change, which either opens an issue or answers with GitHub's own refusal, logged in full.

## Reports carried over from triage

Reports whose `ID` reads `triage-<number>` each stand for an issue in `heliosian/triage`, a private repository of reports. Their columns hold what that issue says: an issue open there is New here, a closed one Dismissed, and their `Issue` points at the triage issue itself rather than at anything in the primary repository. The two of them filed on GitHub by hand rather than through the toolbar name no app and carry no reporter or browser context, since their bodies hold none.

Nothing writes to `heliosian/triage` and nothing reads it.
