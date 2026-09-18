# How Helios Ask answers

Helios Ask has no sheet. It reads every other app's model through `ask.Sources`, which `internal/app` builds from the caches the way the apps' own adapters are built (`internal/app/ask.go`), reads the community's documents through `internal/artifacts` (`artifacts.md`), and talks to Claude through the Anthropic API. This file carries what reading `internal/ask` cannot tell you.

## The call

Each turn is one `Respond`: the two system blocks, the conversation so far with the new message last, the tool definitions, and a way to run a tool. `Claude` in `internal/ask/claude.go` makes it through the beta Messages API, streaming, on `claude-opus-5` with adaptive thinking left on at medium effort, and the server's default fallback for a policy refusal (`fallbacks: "default"` under the `server-side-fallback-2026-07-01` beta), so a question the model declines is re-served by the fallback model inside the same call; one still refused ends in a fixed sentence. Text deltas go to the page as they stream. A `tool_use` stop runs each requested tool here and sends the results back as one user message, for at most eight rounds; the last round closes the tools with `tool_choice: none`, so the turn always ends in words. Every message including the tool exchanges is kept as the conversation, thinking blocks and all, since the same model reads them back.

The prompt's two blocks each carry a cache breakpoint, and the tool definitions sit in front of them, so a conversation's whole prefix is read from the cache from its second turn on until the models change under it. The turn's log line records the input tokens, how many came from the cache, the output tokens, the rounds, the tools run and how long it took, under `ask: answered`, so the cost of a day's questions is a query away (`docs/deploy.md`, Logs).

The key is the one the calendar import and Staff Birthdays already use: `ANTHROPIC_API_KEY`, mirrored locally as `creds/anthropic.key`, and required in real-data mode. It never reaches a browser.

## The conversation

`POST /api/ask/chat` takes `{conversation, message, turns}` - the id the server last gave, the new message, and the transcript the browser holds, as turns of `{role, text, tools}` - and answers `text/event-stream`: `start` with the conversation's id, `text` with each piece of the answer, `tool` with the words for each lookup as it starts, `done` with the turn count, the whole answer and its tools for the browser to keep, and the usage, and `error` when a turn fails - in which case the working copy is left as it was, so the person can send again. `GET /api/ask/model` is the toolbar's user and badges, the starters and the turn limit.

The browser is the record: each chat's turns sit in its local storage under the signed-in address. The server keeps a working copy per conversation in the process, keyed by a random id - the frozen system blocks and the API messages with every tool call, result and thinking block, which is what keeps the cached prefix and lets the model reuse what it already looked up. A working copy is the person's who started it and nobody else's, is dropped an hour after its last message, and is refused a second message while it is still answering (409). When a message names an id the server does not hold - after that hour, after a restart or deploy, or from another person - the server starts a fresh conversation under a new id and rebuilds it from the turns sent along: each question and its answer as plain text, so the model reads what was said and looks things up again when it needs them. The browser takes the new id from `start`. Nothing is written to any sheet or bucket.

Limits, all in code: a message of four thousand characters, fifty chats kept per browser, forty questions per conversation, thirty messages per person per hour, sixteen thousand output tokens per call, eight tool rounds per turn, three minutes per turn, and twenty-four thousand characters per tool result - past which the tool answers with a word on narrowing the search rather than the data.

## The tools

Each tool is a `tool` in `internal/ask`: a name and description for the model, the words the page shows while it runs, an input schema, and a function over a `viewer` - the person and every model as it stood when the turn began, so a refresh mid-answer changes nothing under it. The viewer's address comes from the session, never from the model's arguments; a tool that reads "mine" reads the viewer's.

| Tool | Reads | Keeps back |
| --- | --- | --- |
| `find_people` | The directory by words, role, grade, classroom, department; a parent's grade and classroom are their children's, as Who?'s filters read them | A masked email; a blank phone stays blank |
| `get_person` | One person: card, About Me, families with address and phone as shared, room parent bands | The same masks |
| `get_family` | A family through any member or its name | The same masks |
| `get_classroom` | A classroom: band, grades, teachers, crews with their teachers and students, room parents; all classrooms in brief without a name | Nothing more than the page |
| `calendar_events` | Events in a range (two weeks by default, the whole calendar with a query), the other apps' folded in through the calendar's own `EventsFor`, each with the viewer's answer and household standing; `mine` applies the calendar's own first view through `ViewOf` | Other people's answers |
| `day_plan` | A date's day type and hours per classroom, the events that set it, the school year's span | - |
| `volunteer_opportunities`, `get_activity` | This year's things on HCA-Team and one in full, with spots, co-chairs and volunteers as the portal shows this viewer | Hidden lists (co-chairs and the household alone), pending and hidden things unless the viewer runs them |
| `parties` | The current celebration and every open party still to come with tickets, hosts and audience; the household's tickets; a hosted party's attendees; past parties counted and left out unless asked for | Everyone else's tickets |
| `my_groups` | The groups the viewer manages, the ones open to everyone, and the ones open to members they are on; members for the managed ones | Members of groups they do not manage |
| `my_lists` | The viewer's tags and Magic Tags with their people | Anyone else's tags |
| `community_links` | Heliosian's visible links by section | Hidden links |
| `search_documents` | The community's documents: the passages nearest a question by its embedding and its words, narrowable to a range of dates, each with its document's title, date, age, section and, for a page, its address, and how many documents there are and over what span (`artifacts.md`) | Where a document came from - its kind, channel and author; a passage another already quotes; anything a committee kept to itself, which is never imported |
| `read_document` | One document whole, as markdown, by the key a passage carries, with its address when it has one | Its kind, channel and author |

Every event, party and volunteer thing carries `past` and `whenAgainstToday` ("past (8 days ago)", "today", "tomorrow", "in 12 days"), reckoned here so the model never does date arithmetic, and each of those tools says what today is; a document's `published` is the same words for its date. The words the page shows for a lookup are named once per turn however many times the tool ran. The document search is the one tool that calls out - one embedding request to Vertex AI for the question - and it does so on the turn's context, so a turn that times out ends it too.

The calendar's `ViewOf` and `EventsFor` exist for these tools: the same unexported readings the calendar page makes, opened for another app. Nothing else in the other packages changed for Ask.

## Sample and tests

`Fake` stands in for Claude in sample mode without a key: it runs `day_plan` and streams a canned answer, so the page, the chips and the stream can be tried. The tests in `internal/ask` load every sample sheet the way the server does, with the calendar's roster the directory's classrooms, and pin the prompt for the sample parent, each tool's reading of the sample community, the stream over the fake, that a conversation is its owner's alone, and the limits.
