# Helios Calendar

Helios Calendar is the school year as one family sees it: what kind of day today is for their students' classrooms, what is coming up, and the whole year to browse, with the school's two published sources merged, classified, and corrected in one sheet. It serves at calendar.heliosian.com, and answers as cal.heliosian.com and when.heliosian.com too. The sheet is `Calendar` (`data.md`), the package is `internal/calendar`, and the API is `/api/calendar/`.

## What a viewer sees

Every page is filtered by two choices in the rail, remembered per browser:

- **Classrooms** - which classrooms the pages are about. A parent starts with their children's classrooms, a student with their own, a classroom teacher with theirs, and anyone with none - the office, a specialist - with every classroom. Mine puts a parent's own back; All is the whole school. An event shows when any of its classrooms is chosen; an event with no classroom at all shows to everyone.
- **Show** - which tags. Every tag starts on; None clears them for a quick look at one kind of thing. An event shows when any of its category tags is chosen; the classrooms among its tags are the audience, not a category.

The classrooms and tags are the sheet's live vocabulary, so a renamed classroom drops out of a remembered filter rather than hiding everything.

## Pages

One shell, `web/calendar/index.html`, routes every page on the client, under the shared top bar (`docs/toolbar.md`) beside a rail of the two pages and the filters. On a phone the rail becomes a drawer, filters included, and a bottom tab bar holds the two pages and a way into the filters.

- **Calendar** (`/`, or `/day/{date}` for any other day) - one page in three parts. On the left, the chosen day: a strip of its week, Sunday to Saturday, each day a button, the chosen one filled, today ringed, a day that is not regular carrying a colored mark, with arrows to the weeks either side and a way back to today; then the day plan for the chosen classrooms as one card per day type in force - its name, the classrooms it covers when they are not all of them, and the four blocks, Dropoff, School, Pickup, Aftercare, with their hours, a block the day does not have greyed - a weekend or a day outside the school year saying so instead; then the day's events. On the right, when the window is wide enough, Upcoming in a panel that scrolls on its own: the next special days for the chosen classrooms, then every day ahead that has anything on it, thirty days at a time. Under both, the month grid, paged in place: a day that is not regular wears its day type as a band, with a count when it is only some of the classrooms, then the first few events; today is ringed in amber, days outside the school year are washed, and a cell opens that day above. Narrower windows stack the three.
- **Event** (`/events/{id}`) - the chips (the day type when it carries one, then who it is for and what it is), the title, when, the description with its links live, the day type's blocks when it changes the day, a rail with the date and Add to Google Calendar, the place, where the event came from, and Copy link. Who an event is for is compressed: every classroom is Everyone, both classrooms of a band are the band, the rest are named.
- **Feeds** (`/feeds`) - the viewer's personal calendar feeds and a form for a new one, below.

In every list an event is a row: its hours, its title, the day type it imposes when it does, its place, and who it is for as color - the Config sheet's classroom colors (`docs/config.md`), one dot per color at the row's end and a stripe down its left, and nothing at all for an event that is everyone's. The classroom chips in the rail wear the same colors. Tags show only on the event page.

Search matches every word typed against the title, the description, the place, the tags, and the hidden keywords the classifier wrote, so "half day" finds an early dismissal and "coffee" finds a CAFE. It searches the whole year: the Upcoming panel becomes the matches, grouped by day, and typing on any other page comes back to the calendar with the words.

## Feeds

A feed is an address a calendar app subscribes to: `https://calendar.heliosian.com/feed/{token}.ics`, served without sign-in (`auth.Public`), since Apple Calendar, Google Calendar and Outlook fetch it themselves. The token is the whole secret - 24 characters from a 31-symbol alphabet, minted by the server - so the page says so: anyone holding the address reads the feed, and removing it here is how it stops.

A viewer makes one from the Feeds page by naming it and picking its classrooms (their own to start) and tags (all to start); every classroom or every tag is stored as no filter at all, so a whole-school feed keeps up as classrooms come and go. It lands in the `Feeds` tab with the viewer's address and is theirs alone: the page lists only their own, and only they, or an admin, can remove it. The address is copied to the clipboard the moment it is made. Subscribe in my calendar app is the same address as `webcal://`, which Apple's and most others open straight into a subscription; Add to Google Calendar opens Google's From URL page to paste it into.

The document (`internal/calendar/feed.go`) carries every visible event the feed's filter admits: all-day events as dates with the exclusive end iCalendar wants, timed events in UTC with the school's zone named on the calendar, the tags as categories, the day type at the head of the description, and each event's page as its URL. It asks the client to refresh hourly; Google refreshes on its own schedule, every several hours. The day plan itself is not in the feed: the departures from a regular day are events already, and a regular day is not.

## What it does not do yet

Nothing is edited in the app: the `Events`, `Overrides`, and `Day Overrides` tabs are the input surface, and the `Admins` tab names who will get the editor when it comes. Notifications - a note the week before a short day, an event that moved - are not built; the Change Log the import keeps is shaped for one.

## Sign-in and install

Everything sits behind Google sign-in restricted to the school domain, sharing a session with the other apps on the same tier. It installs to a home screen with the icons under `web/public/calendar/brand/`, which are placeholders drawn in code - a teal tile with a calendar page - until the app has art of its own; the rail and the login page set the name beside the mark in text for the same reason, where the other apps carry a lockup image.
