import {state, me, bands, selectedClassrooms, selectedTags, classroomNames, tagGroups, colorOf} from '../state.js';
import {el, svg, button, copyText, toast} from '../dom.js';
import {setTitle} from '../chrome.js';

// feedTagNames are the categories a feed can carry: every one the page
// has, the built-ins too - a feed carries the parties and HCA events, and
// the owner's household's standing with them.
function feedTagNames() {
  return state.model.tags.map(t => t.name);
}

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

// chip is a pick in the form, the shape of the filter card's: white while
// it holds, dashed and faded while it does not, a classroom's in its color.
function chip(label, on, onClick, color) {
  const b = el('button', 'filter-chip' + (on ? ' is-on' : ''), label);
  b.type = 'button';
  if (color) {
    b.style.setProperty('--room', color);
    b.classList.add('has-color');
  }
  b.addEventListener('click', onClick);
  return b;
}

// newFeedForm picks the feed's classrooms and categories - what the
// calendar's filters show, to start, so Get Feed beside the month makes a
// feed of the view it was pressed on - and names it, then mints it.
function newFeedForm(onMade) {
  const form = el('form', 'feed-form');
  form.id = 'new';
  form.append(el('h2', 'section-title', 'New feed'));
  const rooms = new Set(selectedClassrooms());
  const tags = new Set(selectedTags().filter(t => feedTagNames().includes(t)));
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
        }, colorOf(c.name)));
      }
    }
  };
  paintRooms();
  roomsField.append(roomChips, el('small', '', 'Events for any of these classrooms. Pick every classroom for the whole school.'));
  form.append(roomsField);

  // The categories, one line per group as the filter card has them.
  const tagsField = el('div', 'field');
  tagsField.append(el('span', '', 'Categories'));
  const tagLines = el('div', 'form-tag-groups');
  const paintTags = () => {
    tagLines.replaceChildren();
    for (const group of tagGroups()) {
      const offered = group.tags.filter(t => feedTagNames().includes(t.name));
      if (!offered.length) {
        continue;
      }
      const line = el('div', 'form-tag-group');
      line.append(el('span', 'form-tag-label', group.name || 'Other categories'));
      const chips = el('div', 'filter-chips filter-chips-form');
      for (const t of [...offered].sort((a, b) => a.name.localeCompare(b.name))) {
        const c = chip(t.name, tags.has(t.name), () => {
          if (tags.has(t.name)) {
            tags.delete(t.name);
          } else {
            tags.add(t.name);
          }
          paintTags();
        });
        c.title = t.description;
        chips.append(c);
      }
      line.append(chips);
      tagLines.append(line);
    }
  };
  paintTags();
  tagsField.append(tagLines, el('small', '', 'Events carrying any of these categories.'));
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
      tags: tags.size === feedTagNames().length ? [] : feedTagNames().filter(t => tags.has(t)),
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

  const form = newFeedForm(async made => {
    await copyText(made.url, 'Feed created and its address copied');
    await refreshModel();
  });
  page.append(form);
  // Get Feed on the calendar lands on the form itself.
  if (location.hash === '#new') {
    requestAnimationFrame(() => form.scrollIntoView({block: 'start', behavior: 'smooth'}));
  }
  return page;
}
