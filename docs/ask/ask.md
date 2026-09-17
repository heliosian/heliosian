# Helios Ask

Helios Ask is a chat with Claude for the Helios community: one signed-in member asks about the school, their family, the calendar, the people in the directory, the volunteer work, the parties or the email groups, and the answer streams in, drawn from the community's own apps and linking to them. It serves at ask.heliosian.com. The package is `internal/ask`, the API is `/api/ask/`, and it holds no sheet of its own: everything it knows it reads from the other apps' models as they stand.

## The page

One page. A rail with the brand, New chat, the person's chats newest first with the open one lit and a remove on each, and a word on what the chat is for; the shared toolbar (`docs/toolbar.md`) without a search box, since the composer is the box, and `/` puts the cursor in it; the thread; and the composer pinned under it, a growing box and a send button, Enter sending and Shift-Enter breaking a line.

An empty thread shows the headline over the swoosh, a line on what to ask, and a few starters - questions the server draws from the person's own circumstances ("Who teaches Sam in Jays?") that send as they are. Each question sits on the right in a teal bubble; each answer on the left, drawn from the light markdown Claude writes (paragraphs, lists, bold, links), in the order it happened: the words written before a lookup, a chip naming the lookup ("Looking at the calendar", "Reading a directory page") as it starts, then the words after it; a lookup is named once however many times it ran. While the answer is still thinking, three dots pulse where it will be. Links Claude writes name the apps by their production addresses, and the page lands them on its own tier, so a link to Who? from ask.local.heliosian.com opens who.local.heliosian.com.

The chats live in the browser, under the signed-in address in its local storage - the fifty most recent, each titled by its first question - so they survive a reload, a closed tab, a browser restart and a deploy, and are this browser's and this device's alone. New chat starts another without touching the old ones; opening one from the rail (on a phone, the drawer behind the menu button) picks it up where it stood; the remove forgets it. The server keeps a working copy of each conversation for an hour past its last message, and rebuilds one from the browser's transcript when it no longer has it (`data.md`, The conversation). A chat that reaches forty questions asks for a new one. Someone who sends thirty messages in an hour is asked to wait.

## What Claude knows

The prompt is two cached pieces. The first is the school: `internal/ask/prompt.md`, hand-written and built into the binary - what Helios is, the lingo (classrooms named for birds, bands, crews, room parents, the HCA, the Spring Celebration and the fun(d)raiser parties, Magic Tags, the school year turning over in July), what each app is for and its address, and how to answer - followed by what the models say right now: the grades and their bands, every classroom with its band, grades, teachers and crews, the staff departments, the calendar's categories with the descriptions the Tags tab gives them, the day types with their hours, and the school years. The second is the person asking: their record, their pronouns, each family with every student's grade, classroom, crew and teachers, the bands they are room parent for, the roles the other apps give them (the parties they host, the things they co-chair, the groups they manage) and their own tags by name, and the day and the school year. Both are frozen when the conversation starts, so the cached prefix holds turn after turn; a new chat reads them afresh.

## What Claude can read

Twelve tools, every one read-only, every one reading as the person signed in and never as anyone else (`data.md`, The tools). The directory: find people by words and filters, one person in full with their families, a family, a classroom with its crews and room parents. The calendar: events in a date range with the other apps' folded in and the viewer's own standing on each, and the day plan for a date. HCA-Team: this year's things and who signed up, and one thing in full. Celebrate: the parties with their tickets and the household's own. Loop: the groups the viewer can see, with members for the ones they manage. Who?'s lists: the viewer's tags and Magic Tags. Heliosian: the front page's links.

What the apps keep from a reader, the tools keep too: a masked phone, address or email stays blank, an opted-out person is not there, another person's tags are never read, a hidden volunteer list shows only its co-chairs and the household, a party's attendees reach its hosts alone, a pending or hidden thing reaches whoever runs it. Nothing an admin sees - the super admin list, the admin tabs, provenance, overrides, invoicing - reaches a tool. Staff Birthdays is not read at all.

Claude cannot act: no sign-ups, RSVPs, tickets, mail or edits. The prompt says so, and every answer that wants an action links to the page where the person can take it.

## Sample data

Sample mode signs in as the sample parent, who has two students in two classrooms, co-chairs International Night, manages the soccer team's group and is a room parent, so every tool has something to find. With an Anthropic key in `creds/anthropic.key` the sample server talks to Claude over the fictional community; without one it uses a fake that looks up today's day plan, so the page shows a tool at work, and streams a canned answer a word at a time.

## Brand

The app wears Heliosian's marks as stand-ins under `web/public/ask/brand/` and as its mark in the app switch (`web/public/common/brand/apps/ask.png`), until it has art of its own.
