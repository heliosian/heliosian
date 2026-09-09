# Volunteer portal

The HCA Volunteer Portal is where the Helios Community Association asks for help and where families answer: every event, committee, and idea the association runs each school year, the roles under each, and who signed up. It replaces the Glide app of the same name and serves at hca.heliosian.com. The sheet, package, and API are named for what they hold, `Events`; only the hostname and the branding carry the association's name.

## Entities

- **Category** — a heading on the Sign Up page ("Headline Events", "HCA Committees", "Just an Idea"), with a blurb. Categories display in the order their rows sit in the sheet.
- **Activity** — one thing to help with in one school year: title, category, description, image, when it happens (a start and end, or a free-text timing like "All Year"), where, how many spots, and its status. An activity either takes sign-ups directly or only through its roles.
- **Role** — a committee, task, booth, or shift under an activity. Roles nest: a booth can hold its own performance slot. A role's group is the heading it shows under on the activity page.
- **Volunteer** — a person on an activity or a role, as a volunteer, a volunteer open to co-chairing, or a co-chair, with an optional note.
- **Link** — a sign-up sheet, chat group, or document attached to an activity or a role, shown in a band under its description.

Activities repeat year to year by being copied: "Spring Celebration" in one year and the next are two activities, each with its own roles and sign-ups.

## Status

An activity or role is `Pending`, `Open`, `Done`, or `Hidden`. Pending is a suggestion waiting for an admin; open takes sign-ups; done is over; hidden is parked. Whether something is full is not a status: a spots count sets the capacity, and the sign-up count against it says the rest.

## Who may do what

- **Anyone signed in** browses everything open, signs themselves or someone else up, removes themselves, suggests an activity or a role (which lands pending), and sees the co-chairs of a private list plus their own entry on it.
- **Editors of an activity**, its admins and co-chairs, change it and its roles and links, set statuses short of approving, name co-chairs, see hidden volunteer lists, remove anyone, and copy the volunteer emails.
- **Admins**, the Admins tab plus the platform super admins (`docs/config.md`), approve suggestions, hide and delete, copy an activity into the next year, and manage categories, settings, and the admin list from `/admin`.

## Pages

One shell, `web/hca/index.html`, routes every page on the client.

- **Sign Up** (`/`) — the intro, the expense form and Suggest an Idea buttons, and tabs for this school year, last year, and (for admins) what needs approval. Each year lists its categories in sheet order, activities within a category by start date with undated ones after, and a Show Previous Events switch that brings back what is done or past. Another year is `/years/{year}`.
- **My Activities** (`/my`) — the viewer's own sign-ups by year and category, each as Activity ▶ Role.
- **HCA Calendar** (`/calendar`) — a month grid of every dated activity and role.
- **Activity** (`/activities/{year}/{title}`) — the image, when and where, description, link band, Add to Calendar, co-chairs, the sign-up band, and tabs for its roles (grouped), its volunteers, and, for editors, the hidden and pending roles. Editors get a control band: status, the flags, and the edit, link, role, copy-emails, and copy-to-next-year actions.
- **Role** (`.../roles/{title}`) — the same shape one level down, with the roles under it.
- **All Activities** (`/all`), **People** (`/people`), and **Admin Tools** (`/admin`) — admin pages: every year with Copy to Next Year on each activity, sign-up counts per person, and the categories, settings, and admin list.

On phones the top bar carries the page title and a drawer for the rest, and a bottom tab bar holds Sign Up, My Activities, and HCA Calendar, matching the app this replaces.

## The school year

School years turn over on July 1 and are written as both calendar years, `2026 - 2027`. The current, last, and next years are computed from the date; nothing records them.

## Names and photos

The portal stores only email addresses. Names and photos come from the directory model at request time, so a volunteer looks the same here as in Helios Who?. Someone the directory does not list, such as a parent who has not opted in, is shown under a name read out of their address. Sign-in resolves through the directory's email aliases too, so a person is one address across every app.

## Editing

As in the other apps: in place, through modals, with every change applied to the in-memory model first and then written to the sheet cell by cell through the shared write queue and appended to the Change Log. A change the rules reject is refused before anything is written. Deleting an activity or role refuses while anyone is signed up for it, and deleting a category refuses while an activity names it.

## Sign-in and install

Everything sits behind Google sign-in restricted to the school domain, sharing a session with the other apps on the same tier. It installs to a home screen like the directory (`docs/who/pwa.md`), with the icons and splash screens the Glide app used under `web/public/hca/`.
