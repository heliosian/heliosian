import {state, defaultFeedName, feedURL, webcalURL, bands, selectedClassrooms, selectedTags, classroomNames, tagGroups, colorOf} from '../state.js';
import {el, svg, button, copyText, toast, feedMark, emojiPicker} from '../dom.js';
import {setTitle} from '../chrome.js';

// feedTagNames are the categories a feed can carry: every one the page
// has, the built-ins too - a feed carries the parties and HCA events, and
// the owner's household's standing with them.
function feedTagNames() {
  return state.model.tags.map(t => t.name);
}

// showsLine is what a feed carries, as small chips under its address:
// its classrooms in their colors, then its categories - or a word for
// every one where it carries no filter.
function showsLine(f) {
  const line = el('div', 'feed-shows');
  line.append(el('span', 'feed-shows-label', 'Shows'));
  if (f.classrooms.length) {
    for (const c of f.classrooms) {
      const chip = el('span', 'feed-chip has-color', c);
      chip.style.setProperty('--room', colorOf(c));
      line.append(chip);
    }
  } else {
    line.append(el('span', 'feed-chip feed-chip-all', 'Every classroom'));
  }
  line.append(el('span', 'feed-shows-sep'));
  if (f.tags.length) {
    for (const t of f.tags) {
      line.append(el('span', 'feed-chip', t));
    }
  } else {
    line.append(el('span', 'feed-chip feed-chip-all', 'Every category'));
  }
  return line;
}

async function refreshModel() {
  const {load} = await import('../app.js');
  await load();
}

// createdWords is when a feed was made, from its stamp ("2026-09-14 08:06").
function createdWords(f) {
  const [y, m, d] = (f.created || '').slice(0, 10).split('-').map(Number);
  if (!y) {
    return '';
  }
  return 'Made ' + new Date(y, m - 1, d).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'});
}

function feedCard(f) {
  const card = el('div', 'feed-card');
  const head = el('div', 'feed-head');
  // Edit is the pencil beside the name: it opens the feed's own form under
  // the card - its name, classrooms and categories as they stand - and a
  // save keeps the address.
  const editor = el('div', 'feed-editor');
  editor.hidden = true;
  const nameRow = el('div', 'feed-name-row');
  const openEditor = () => {
    if (!editor.hidden) {
      editor.hidden = true;
      return;
    }
    editor.replaceChildren(feedForm({feed: f, onDone: async () => {
      toast('Feed updated');
      await refreshModel();
    }, onCancel: () => {
      editor.hidden = true;
    }}));
    editor.hidden = false;
    editor.querySelector('input').focus();
  };
  const pencil = button('', 'pencil', 'feed-edit', openEditor);
  pencil.title = 'Edit this feed';
  pencil.setAttribute('aria-label', 'Edit this feed');
  nameRow.append(feedMark(f), el('div', 'feed-name', f.name), pencil);
  head.append(nameRow);
  if (createdWords(f)) {
    head.append(el('div', 'feed-made', createdWords(f)));
  }
  card.append(head);
  const url = el('div', 'feed-url');
  const input = el('input');
  input.type = 'text';
  input.readOnly = true;
  input.value = feedURL(f.token);
  input.addEventListener('focus', () => input.select());
  url.append(input);
  url.append(button('Copy', 'copy', 'button button-small', () => copyText(feedURL(f.token), 'Feed address copied')));
  // The same form opens from Edit at the end of the Shows row.
  const shows = showsLine(f);
  shows.append(button('Edit', 'pencil', 'feed-shows-edit', openEditor));
  card.append(url, shows);
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
  card.append(actions, editor);
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

// feedForm names a feed and picks its classrooms and categories. A new
// feed starts from what the calendar's filters show, so Save Calendar beside
// the month makes a feed of the view it was pressed on, and is minted on
// submit; an existing one (feed given) starts from its own picks - every
// classroom or category where it carries no filter - and is changed in
// place, keeping its address.
function feedForm({feed = null, onDone, onCancel = null} = {}) {
  const form = el('form', 'feed-form');
  if (!feed) {
    form.id = 'new';
    form.append(el('h2', 'section-title', 'New feed'));
  }
  const rooms = new Set(feed ? (feed.classrooms.length ? feed.classrooms : classroomNames()) : selectedClassrooms());
  const tags = new Set(feed ? (feed.tags.length ? feed.tags : feedTagNames()) : selectedTags().filter(t => feedTagNames().includes(t)));
  const nameField = el('label', 'field');
  nameField.append(el('span', '', 'Name'));
  const name = el('input');
  name.type = 'text';
  name.maxLength = 80;
  name.value = feed ? feed.name : defaultFeedName();
  nameField.append(name);
  form.append(nameField);

  // The mark before the name in the rail: an emoji typed or picked, or
  // none for the calendar icon.
  const emojiField = el('div', 'field');
  emojiField.append(el('span', '', 'Emoji'));
  const {node: emojiRow, input: emoji} = emojiPicker(feed ? feed.emoji || '' : '');
  emojiField.append(emojiRow, el('small', '', 'Shown before the name under Calendar in the rail; none means the calendar icon.'));
  form.append(emojiField);

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
  submit.append(svg(feed ? 'check' : 'plus'), el('span', '', feed ? 'Save changes' : 'Create feed'));
  actions.append(submit);
  if (onCancel) {
    actions.append(button('Cancel', '', 'button button-secondary', onCancel));
  }
  actions.append(status);
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
    status.textContent = feed ? 'Saving…' : 'Creating…';
    const body = {
      name: name.value.trim() || defaultFeedName(),
      emoji: emoji.value.trim(),
      classrooms: rooms.size === classroomNames().length ? [] : classroomNames().filter(c => rooms.has(c)),
      tags: tags.size === feedTagNames().length ? [] : feedTagNames().filter(t => tags.has(t)),
    };
    if (feed) {
      body.token = feed.token;
    }
    const res = await fetch('/api/calendar/feeds', {method: feed ? 'PUT' : 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
    submit.disabled = false;
    if (!res.ok) {
      status.textContent = await res.text();
      status.classList.add('error');
      return;
    }
    status.textContent = '';
    onDone(feed ? null : await res.json());
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
  // New feed opens the form, below the list; it stays out of the way
  // until then, unless Save Calendar on the calendar sent the viewer here.
  const holder = el('div', 'feed-new');
  holder.hidden = true;
  const form = feedForm({onDone: async made => {
    await copyText(made.url, 'Feed created and its address copied');
    await refreshModel();
  }, onCancel: () => {
    holder.hidden = true;
  }});
  holder.append(form);
  const openNew = () => {
    holder.hidden = false;
    requestAnimationFrame(() => form.scrollIntoView({block: 'start', behavior: 'smooth'}));
    form.querySelector('input').focus();
  };
  head.append(button('New feed', 'plus', 'button', openNew));
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

  page.append(holder);
  // Save Calendar on the calendar can land on the form itself.
  if (location.hash === '#new') {
    openNew();
  }
  return page;
}
