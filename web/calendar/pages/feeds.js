import {state, me, bands, myClassrooms, classroomNames, tagNames} from '../state.js';
import {el, svg, button, copyText, toast} from '../dom.js';
import {setTitle} from '../chrome.js';

function feedURL(token) {
  return `${location.origin}/feed/${token}.ics`;
}

function webcalURL(token) {
  return `webcal://${location.host}/feed/${token}.ics`;
}

function filterWords(f) {
  const rooms = f.classrooms.length ? f.classrooms.join(', ') : 'Every classroom';
  const tags = f.tags.length ? f.tags.join(', ') : 'Everything';
  return `${rooms} · ${tags}`;
}

async function refreshModel() {
  const {load} = await import('../app.js');
  await load();
}

function feedCard(f) {
  const card = el('div', 'feed-card');
  const head = el('div', 'feed-head');
  head.append(el('div', 'feed-name', f.name), el('div', 'feed-filter', filterWords(f)));
  card.append(head);
  const url = el('div', 'feed-url');
  const input = el('input');
  input.type = 'text';
  input.readOnly = true;
  input.value = feedURL(f.token);
  input.addEventListener('focus', () => input.select());
  url.append(input);
  url.append(button('Copy', 'copy', 'button button-small', () => copyText(feedURL(f.token), 'Feed address copied')));
  card.append(url);
  const actions = el('div', 'feed-actions');
  const open = el('a', 'button button-secondary button-small');
  open.href = webcalURL(f.token);
  open.append(svg('calendar'), el('span', '', 'Subscribe in my calendar app'));
  actions.append(open);
  const google = el('a', 'button button-secondary button-small');
  google.href = 'https://calendar.google.com/calendar/u/0/r/settings/addbyurl';
  google.target = '_blank';
  google.rel = 'noopener';
  google.append(svg('open'), el('span', '', 'Add to Google Calendar'));
  actions.append(google);
  actions.append(button('Remove', 'trash', 'link-button danger', async () => {
    if (!confirm(`Remove the feed "${f.name}"? Calendars subscribed to it stop updating.`)) {
      return;
    }
    const res = await fetch('/api/calendar/feeds', {method: 'DELETE', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({token: f.token})});
    if (!res.ok) {
      toast(await res.text());
      return;
    }
    toast('Feed removed');
    await refreshModel();
  }));
  card.append(actions);
  return card;
}

function chip(label, on, onClick) {
  const b = el('button', 'filter-chip' + (on ? ' is-on' : ''), label);
  b.type = 'button';
  b.addEventListener('click', onClick);
  return b;
}

// newFeedForm picks the feed's classrooms and tags - the viewer's own
// classrooms and everything to start - and names it, then mints it.
function newFeedForm(onMade) {
  const form = el('form', 'feed-form');
  form.append(el('h2', 'section-title', 'New feed'));
  const rooms = new Set(myClassrooms().length ? myClassrooms() : classroomNames());
  const tags = new Set(tagNames());
  const nameField = el('label', 'field');
  nameField.append(el('span', '', 'Name'));
  const name = el('input');
  name.type = 'text';
  name.maxLength = 80;
  name.value = 'Helios Calendar';
  nameField.append(name);
  form.append(nameField);

  const roomsField = el('div', 'field');
  roomsField.append(el('span', '', 'Classrooms'));
  const roomChips = el('div', 'filter-chips filter-chips-form');
  const paintRooms = () => {
    roomChips.replaceChildren();
    for (const band of bands()) {
      for (const c of band.classrooms) {
        roomChips.append(chip(c.name, rooms.has(c.name), () => {
          if (rooms.has(c.name)) {
            rooms.delete(c.name);
          } else {
            rooms.add(c.name);
          }
          paintRooms();
        }));
      }
    }
  };
  paintRooms();
  roomsField.append(roomChips, el('small', '', 'Events for any of these classrooms. Pick every classroom for the whole school.'));
  form.append(roomsField);

  const tagsField = el('div', 'field');
  tagsField.append(el('span', '', 'Show'));
  const tagChips = el('div', 'filter-chips filter-chips-form');
  const paintTags = () => {
    tagChips.replaceChildren();
    for (const t of state.model.tags) {
      const c = chip(t.name, tags.has(t.name), () => {
        if (tags.has(t.name)) {
          tags.delete(t.name);
        } else {
          tags.add(t.name);
        }
        paintTags();
      });
      c.title = t.description;
      tagChips.append(c);
    }
  };
  paintTags();
  tagsField.append(tagChips, el('small', '', 'Events carrying any of these tags.'));
  form.append(tagsField);

  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('plus'), el('span', '', 'Create feed'));
  actions.append(submit, status);
  form.append(actions);
  form.addEventListener('submit', async e => {
    e.preventDefault();
    if (!rooms.size) {
      status.textContent = 'Pick at least one classroom.';
      status.classList.add('error');
      return;
    }
    if (!tags.size) {
      status.textContent = 'Pick at least one tag.';
      status.classList.add('error');
      return;
    }
    submit.disabled = true;
    status.classList.remove('error');
    status.textContent = 'Creating…';
    const body = {
      name: name.value.trim() || 'Helios Calendar',
      classrooms: rooms.size === classroomNames().length ? [] : classroomNames().filter(c => rooms.has(c)),
      tags: tags.size === tagNames().length ? [] : tagNames().filter(t => tags.has(t)),
    };
    const res = await fetch('/api/calendar/feeds', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
    submit.disabled = false;
    if (!res.ok) {
      status.textContent = await res.text();
      status.classList.add('error');
      return;
    }
    status.textContent = '';
    onMade(await res.json());
  });
  return form;
}

export function feedsPage() {
  setTitle('Feeds');
  const page = el('div');
  const head = el('div', 'page-head');
  const main = el('div', 'page-head-main');
  main.append(el('h1', 'page-title', 'Calendar Feeds'));
  main.append(el('p', 'page-intro', 'A feed is an address your own calendar app subscribes to. It carries the events you choose and keeps itself up to date as the school calendar changes.'));
  head.append(main);
  page.append(head);

  const mine = state.model.feeds;
  page.append(el('h2', 'section-title', mine.length ? 'My feeds' : 'No feeds yet'));
  if (mine.length) {
    const list = el('div', 'feed-list');
    for (const f of mine) {
      list.append(feedCard(f));
    }
    page.append(list);
  }

  const help = el('div', 'panel help-panel');
  help.append(el('div', 'help-title', 'How to subscribe'));
  const steps = el('ul', 'help-list');
  for (const words of [
    'Apple Calendar (iPhone, iPad, Mac): Subscribe in my calendar app opens it straight away. Or File › New Calendar Subscription and paste the address.',
    'Google Calendar: open Add to Google Calendar, paste the address under From URL, and Add calendar. Google refreshes a subscribed calendar every few hours.',
    'Outlook: Add calendar › Subscribe from web, and paste the address.',
    'The address is the whole secret: anyone who has it can read the feed. Remove a feed here and it stops.',
  ]) {
    steps.append(el('li', '', words));
  }
  help.append(steps);
  page.append(help);

  page.append(newFeedForm(async made => {
    await copyText(made.url, 'Feed created and its address copied');
    await refreshModel();
  }));
  return page;
}
