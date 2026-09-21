import {myEvents} from '../state.js';
import {el} from '../dom.js';
import {setTitle} from '../chrome.js';
import {eventRow} from '../events.js';

// minePage is My Events: the viewer's own standing with what is coming
// up, a group per standing - the invitations waiting for their reply, the
// events they host, the events they said yes to - each event a row to its
// page, as the lists have them.
export function minePage() {
  setTitle('My Events');
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'My Events'));
  main.append(el('p', 'page-intro', 'The invitations waiting for your reply, the events you host, and the ones you said yes to. Each opens its page, where the RSVPs are.'));
  head.append(main);
  page.append(head);
  const mine = myEvents();
  const groups = [
    ['Waiting for your reply', mine.waiting, 'is-waiting', 'Nothing waiting on you.'],
    ['Hosting', mine.hosted, 'is-hosted', 'Nothing you host is coming up. Add Event, under the day, starts one.'],
    ['Going', mine.going, 'is-going', 'Nothing you said yes to is coming up.'],
  ];
  for (const [label, events, cls, empty] of groups) {
    const group = el('section', 'mine-group');
    group.append(el('div', 'rsvps-head ' + cls, events.length ? `${label} · ${events.length}` : label));
    if (!events.length) {
      group.append(el('p', 'mine-empty', empty));
    } else {
      const list = el('div', 'event-list');
      for (const e of events) {
        list.append(eventRow(e, {showDate: true}));
      }
      group.append(list);
    }
    page.append(group);
  }
  return page;
}
