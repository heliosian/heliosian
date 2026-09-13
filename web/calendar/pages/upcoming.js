import {state, eventsOn, today, addDays, nextSpecials} from '../state.js';
import {el, button} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {eventRow, dayHeading, specialRow, emptyNote} from '../events.js';

const stretch = 30;

let shown = stretch;

// upcomingPage lists the days ahead that have something on them, the plan
// chips beside each date, and the next departures from the regular day up
// front so a short day is never a surprise.
export function upcomingPage() {
  setTitle('Upcoming');
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Upcoming'));
  head.append(main);
  page.append(head);

  const specials = nextSpecials(addDays(today(), -1), 6);
  if (specials.length) {
    page.append(el('h2', 'section-title', 'Special days ahead'));
    const list = el('div', 'special-list');
    for (const item of specials) {
      list.append(specialRow(item));
    }
    page.append(list);
  }

  const body = el('div');
  const paint = () => {
    body.replaceChildren();
    body.append(el('h2', 'section-title', state.query ? 'Matching events' : `The next ${shown} days`));
    let any = false;
    const from = today();
    const until = state.query ? addDays(from, 400) : addDays(from, shown);
    for (let date = from; date < until; date = addDays(date, 1)) {
      const events = eventsOn(date);
      if (!events.length) {
        continue;
      }
      any = true;
      const day = el('div', 'day-group');
      day.append(dayHeading(date));
      const list = el('div', 'event-list');
      for (const e of events) {
        list.append(eventRow(e));
      }
      day.append(list);
      body.append(day);
    }
    if (!any) {
      body.append(emptyNote(state.query ? 'Nothing matches.' : 'Nothing on the calendar in this stretch for these classrooms and tags.'));
    }
    if (!state.query) {
      body.append(button(`Show the next ${stretch} days too`, 'plus', 'button button-secondary', () => {
        shown += stretch;
        paint();
      }));
    }
  };
  paint();
  page.append(body);
  setSearch('Search upcoming events…', q => {
    state.query = q;
    paint();
  });
  return page;
}
