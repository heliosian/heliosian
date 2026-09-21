// The date card every app's event page floats over its banner: a cream
// tile for the day - weekday, month, day, and the sun as it stands when
// the event starts - a second, pale-green tile with the sun as it stands
// at the end when the event runs across days, then a rule and the facts
// in lines: the hours with a clock and the place with a pin, or for a
// span each end with its day and how many days it runs. Its styles are
// datecard.css. dateCard takes the app's el, and the event's start and
// end as the sheets write a moment ("2026-10-10 15:00", or a bare date),
// whether it is all day, and its place.

const icons = {
  clock: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18zM12 7v5l3 2',
  pin: 'M12 22s7-7.6 7-12a7 7 0 1 0-14 0c0 4.4 7 12 7 12zM12 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
  calendar: 'M4 5h16a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM3 10h18M8 3v4M16 3v4',
  // A calendar with a plus at its corner, for Add.
  calendarPlus: 'M12 20H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h16a1 1 0 0 1 1 1v6M3 10h18M8 3v4M16 3v4M18 15v6M15 18h6',
};

// icon is one of the marks above as an inline svg.
function icon(name) {
  const mark = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  mark.setAttribute('viewBox', '0 0 24 24');
  mark.setAttribute('fill', 'none');
  mark.setAttribute('stroke', 'currentColor');
  mark.setAttribute('stroke-width', '1.8');
  mark.setAttribute('stroke-linecap', 'round');
  mark.setAttribute('stroke-linejoin', 'round');
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', icons[name]);
  mark.append(path);
  return mark;
}

// The sun on a tile says the time of day: rising with its rays for a
// morning, high and whole for an afternoon, setting over its horizon for
// an evening, and a moon for the night.
const sunArt = {
  morning: '<svg viewBox="0 0 40 24" aria-hidden="true"><g stroke="#f5b400" stroke-width="2.4" stroke-linecap="round"><path d="M20 3v4M8.5 7.5l2.8 2.8M31.5 7.5l-2.8 2.8M3 19h5M32 19h5"/></g><path d="M11 21a9 9 0 0 1 18 0z" fill="#f5b400"/></svg>',
  afternoon: '<svg viewBox="0 0 40 24" aria-hidden="true"><g stroke="#f5b400" stroke-width="2.2" stroke-linecap="round"><path d="M20 1.5v3M20 19.5v3M8.5 3.5l2.2 2.2M29.3 18.3l2.2 2.2M8.5 20.5l2.2-2.2M29.3 5.7l2.2-2.2M1.5 12h3M35.5 12h3"/></g><circle cx="20" cy="12" r="6.5" fill="#f5b400"/></svg>',
  evening: '<svg viewBox="0 0 40 24" aria-hidden="true"><path d="M10 15a10 10 0 0 1 20 0z" fill="#f28a1b"/><g stroke="#f28a1b" stroke-width="2.4" stroke-linecap="round"><path d="M6 18.5h28M11 22.5h18"/></g></svg>',
  night: '<svg viewBox="0 0 40 24" aria-hidden="true"><path d="M23 2.5a10 10 0 1 0 8.5 15.2A9 9 0 0 1 23 2.5z" fill="#8fa3c8"/><circle cx="9" cy="6" r="1.3" fill="#8fa3c8"/><circle cx="13" cy="15" r="1" fill="#8fa3c8"/></svg>',
};

const weekdayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short'});
const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'short'});
const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
const clockFormat = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

// parseWhen reads a moment as the sheets write one: a date, with or
// without a clock.
export function parseWhen(s) {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2}))?/.exec(s || '');
  if (!m) {
    return null;
  }
  return {date: new Date(+m[1], m[2] - 1, +m[3], m[4] ? +m[4] : 0, m[5] ? +m[5] : 0), hasTime: Boolean(m[4])};
}

// timeOfDay is the sun for a moment: before noon a morning, before five
// an afternoon, before nine an evening, else the night; a day with no
// clock takes the afternoon's whole sun.
export function timeOfDay(when) {
  if (!when || !when.hasTime) {
    return 'afternoon';
  }
  const h = when.date.getHours();
  return h < 12 ? 'morning' : h < 17 ? 'afternoon' : h < 21 ? 'evening' : 'night';
}

// timeRange is two clocks as one span, AM or PM written once when both
// share it: "8:00 – 8:15 AM", "11:00 AM – 2:00 PM".
export function timeRange(from, to) {
  const a = clockFormat.format(from);
  const b = clockFormat.format(to);
  const half = t => t.slice(-2);
  if (half(a) === half(b)) {
    return `${a.slice(0, -3)} – ${b}`;
  }
  return `${a} – ${b}`;
}

function sameDay(a, b) {
  return a.date.toDateString() === b.date.toDateString();
}

// googleCalendarLink is the event as a Google Calendar template link -
// an hour long when it has a start and no end, the whole day when it has
// no clock - the details carrying the words and the page's address.
export function googleCalendarLink({title, start, end, allDay = false, location = '', details = ''}) {
  const from = parseWhen(start);
  if (!from) {
    return '';
  }
  const to = parseWhen(end);
  const timed = !allDay && from.hasTime;
  const stamp = d => `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`
    + (timed ? `T${String(d.getHours()).padStart(2, '0')}${String(d.getMinutes()).padStart(2, '0')}00` : '');
  let until = to && to.date > from.date ? to.date : new Date(from.date.getTime() + 60 * 60 * 1000);
  if (!timed) {
    until = new Date((to ? to.date : from.date).getTime() + 24 * 60 * 60 * 1000);
  }
  const params = new URLSearchParams({action: 'TEMPLATE', text: title, dates: `${stamp(from.date)}/${stamp(until)}`, details, location});
  return 'https://calendar.google.com/calendar/render?' + params.toString();
}

// dateCard draws the card; add, when given, is the address behind Add at
// the end of the hours line - a calendar with a plus, and the word - which
// puts the event on the reader's Google Calendar.
export function dateCard(el, {start, end, allDay = false, location = '', add = ''}) {
  const from = parseWhen(start);
  if (!from) {
    return null;
  }
  const to = parseWhen(end) || from;
  const span = !sameDay(from, to);
  const timed = !allDay && from.hasTime;
  const card = el('div', 'hero-date' + (span ? ' is-span' : ''));
  const tile = (when, cls) => {
    const t = el('div', 'hero-tile ' + cls);
    t.append(el('span', 'hero-dow', weekdayFormat.format(when.date)), el('span', 'hero-mon', monthFormat.format(when.date)), el('span', 'hero-num', String(when.date.getDate())));
    const sun = el('span', 'hero-sun');
    sun.innerHTML = sunArt[timed ? timeOfDay(when) : 'afternoon'];
    t.append(sun);
    return t;
  };
  card.append(tile(from, 'is-start'));
  if (span) {
    card.append(el('span', 'hero-dash', '–'), tile(to, 'is-end'));
  }
  const lines = el('div', 'hero-lines');
  const line = (name, ...words) => {
    const row = el('div', 'hero-line');
    row.append(icon(name));
    const text = el('div', 'hero-line-text');
    for (const w of words) {
      text.append(el('div', '', w));
    }
    row.append(text);
    return row;
  };
  // Add sits at the end of the first line, the hours'.
  const withAdd = row => {
    if (add) {
      const button = el('a', 'hero-add');
      button.href = add;
      button.target = '_blank';
      button.rel = 'noopener';
      button.title = 'Add to Google Calendar';
      button.append(icon('calendarPlus'), el('span', '', 'Add'));
      row.append(button);
    }
    return row;
  };
  if (span) {
    const at = (when, lead) => lead + dayFormat.format(when.date) + (timed && when.hasTime ? ` at ${clockFormat.format(when.date)}` : '');
    lines.append(withAdd(line('clock', at(from, ''), at(to, 'to '))));
    const days = Math.round((new Date(to.date.getFullYear(), to.date.getMonth(), to.date.getDate()) - new Date(from.date.getFullYear(), from.date.getMonth(), from.date.getDate())) / 86400000) + 1;
    lines.append(line('calendar', `${days} days`));
  } else {
    const hours = !timed ? 'All day' : to.hasTime && to.date > from.date ? timeRange(from.date, to.date) : clockFormat.format(from.date);
    lines.append(withAdd(line('clock', hours)));
    if (location) {
      lines.append(line('pin', location));
    }
  }
  card.append(el('span', 'hero-sep'), lines);
  return card;
}
