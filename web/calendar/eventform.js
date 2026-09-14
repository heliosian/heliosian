import {state, me, tagGroups, bands, classroomNames, eventDates, addDays, parseDate} from './state.js';
import {el, svg, toast} from './dom.js';
import {imageControl} from './images.js';

// randomID is an event's address to start with, the shape the server
// mints: eight letters and digits, the confusable ones left out.
function randomID() {
  const alphabet = 'abcdefghjkmnpqrstvwxyz0123456789';
  const raw = new Uint8Array(8);
  crypto.getRandomValues(raw);
  return [...raw].map(b => alphabet[b % alphabet.length]).join('');
}

// eventForm is the form that adds an event, for an admin and for anyone
// else: two tabs, the event itself and the tags it is filed under, every
// field staying in the form so nothing typed is lost in switching. An
// admin's event goes straight onto the calendar; anyone else's is shared,
// and waits for an admin's approval. Filled from an event to clone when
// there is one, its dates moved on by the weeks asked.
export function eventForm({from = null, shift = 0, edit = null, onDone}) {
  const admin = me().isAdmin;
  // Editing fills from the event itself, dates as they are.
  if (edit) {
    from = edit;
    shift = 0;
  }
  const form = el('form', 'admin-form');
  form.append(el('p', 'hint', edit ? 'Change what you need to; the event keeps its place on the calendar.' : admin ? 'Goes into the Events tab as the community\u2019s own, under your name.' : 'Shared under your name. An admin approves it onto the calendar; until then only you and the admins see it, and you will see who answers it.'));
  // Two tabs: the event itself, and the tags it is filed under. Every
  // field stays in the form - only the panels hide - so nothing typed is
  // lost in switching.
  const tabs = el('div', 'tabs');
  const eventPanel = el('div');
  const tagPanel = el('div');
  tagPanel.hidden = true;
  // The first tab ends in Next, the second in the button that adds.
  let showPanel = null;
  const tabButton = (label, panel) => {
    const b = el('button', 'tab-button', label);
    b.type = 'button';
    b.addEventListener('click', () => showPanel(panel));
    return b;
  };
  const eventTab = tabButton('Event', eventPanel);
  const tagTab = tabButton('Tags', tagPanel);
  showPanel = panel => {
    eventTab.classList.toggle('is-active', panel === eventPanel);
    tagTab.classList.toggle('is-active', panel === tagPanel);
    eventPanel.hidden = panel !== eventPanel;
    tagPanel.hidden = panel !== tagPanel;
  };
  tabs.append(eventTab, tagTab);
  form.append(tabs, eventPanel, tagPanel);
  showPanel(eventPanel);
  const field = (label, input, note) => {
    const wrap = el('label', 'field');
    wrap.append(el('span', '', label), input);
    if (note) {
      wrap.append(el('small', '', note));
    }
    return wrap;
  };
  const text = (value, placeholder) => {
    const input = el('input');
    input.type = 'text';
    input.value = value || '';
    input.placeholder = placeholder || '';
    return input;
  };
  const title = text(from ? from.title : '');
  title.required = true;
  title.maxLength = 200;
  eventPanel.append(field('Title', title));

  // Who can find it: everyone, on the calendar (an admin approves it
  // first, for anyone but an admin), or only those given its direct link,
  // who answer it onto their own calendars.
  let inviteOnly = Boolean(from && from.source === 'sheet' && from.inviteOnly);
  const whoField = el('div', 'field');
  whoField.append(el('span', '', 'Who can find it'));
  const choices = el('div', 'event-visibility');
  const choice = (label, note, value) => {
    const b = el('button', 'event-visibility-choice' + (inviteOnly === value ? ' is-on' : ''));
    b.type = 'button';
    b.append(el('span', 'event-visibility-title', label), el('span', 'event-visibility-note', note));
    b.addEventListener('click', () => {
      inviteOnly = value;
      for (const c of choices.children) {
        c.classList.toggle('is-on', c === b);
      }
    });
    return b;
  };
  choices.append(
    choice('Public', admin ? 'On the calendar for everyone at Helios to discover.' : 'On the calendar for everyone at Helios to discover. Public events require admin approval, but you can share the link right away directly.', false),
    choice('Direct link only', 'Only people you send the link to. Their yes or no puts it on their calendar.', true),
  );
  whoField.append(choices);
  eventPanel.append(whoField);

  // The event's web address: a random one to start, or one the host
  // types - letters, digits and dashes - for a link worth sending. Set
  // once; an event keeps its address after that.
  let slug = null;
  if (!edit) {
    slug = el('input', 'event-slug');
    slug.type = 'text';
    slug.maxLength = 40;
    slug.value = randomID();
    slug.spellcheck = false;
    slug.addEventListener('input', () => {
      slug.value = slug.value.toLowerCase().replace(/[^a-z0-9-]/g, '');
    });
    const row = el('div', 'event-slug-row');
    row.append(el('span', 'event-slug-prefix', location.host + '/e/'), slug);
    const again = el('button', 'link-button');
    again.type = 'button';
    again.textContent = 'New random one';
    again.addEventListener('click', () => {
      slug.value = randomID();
    });
    row.append(again);
    const slugField = el('div', 'field');
    slugField.append(el('span', '', 'Web address'), row, el('small', '', 'Letters, digits and dashes. Make it your own, or keep the random one.'));
    eventPanel.append(slugField);
  }

  // When: a date and, unless it is all day, a time, for the start and the
  // end. A cloned event's dates move on by the weeks asked.
  const startDate = el('input');
  startDate.type = 'date';
  startDate.required = true;
  const startTime = el('input');
  startTime.type = 'time';
  const endDate = el('input');
  endDate.type = 'date';
  const endTime = el('input');
  endTime.type = 'time';
  if (from) {
    const first = eventDates(from)[0];
    const last = eventDates(from)[eventDates(from).length - 1];
    startDate.value = addDays(first, 7 * shift);
    endDate.value = addDays(last, 7 * shift);
    if (!from.allDay) {
      startTime.value = from.start.slice(11, 16);
      endTime.value = from.end.slice(11, 16);
    }
  }
  // Moving the start moves the end with it, keeping the span - so a copy
  // dragged to another day stays as long as it was.
  let lastStart = startDate.value;
  startDate.addEventListener('change', () => {
    if (lastStart && startDate.value && endDate.value) {
      const days = Math.round((parseDate(startDate.value) - parseDate(lastStart)) / 86400000);
      endDate.value = addDays(endDate.value, days);
    }
    lastStart = startDate.value;
  });
  const whenRow = el('div', 'admin-when');
  whenRow.append(field('Starts', startDate), field('At', startTime, 'Leave blank for all day'), field('Ends', endDate, 'Blank means the same day'), field('Until', endTime));
  eventPanel.append(whenRow);
  const place = text(from ? from.location : '');
  eventPanel.append(field('Location', place));
  const source = text(from ? from.sourceUrl || from.sourceNote || '' : '', 'https://…');
  eventPanel.append(field('More Information or Source', source, admin ? 'Where this came from, or where to read more - a web address links from the event\u2019s page; any other words are shown as written.' : 'A web address with more about it, if there is one.'));
  const description = el('textarea');
  description.rows = 4;
  description.value = from ? from.description || '' : '';
  eventPanel.append(field('Description', description));

  // A picture across the top of the event's page, with the picker the
  // categories use: an upload, a search of the picture libraries, or none
  // for its first category's, or the calendar's own.
  const picture = {name: from ? from.title : '', image: from && from.source === 'sheet' && from.image ? from.image.replace(/^\//, '') : '', imageUrl: from && from.source === 'sheet' ? from.image || '' : ''};
  // Not asked for up front: a small link opens the field, unless the
  // event already has a picture.
  const imageField = el('div', 'field');
  imageField.append(el('span', '', 'Picture'));
  const imageRow = el('div', 'event-image-row');
  const paintImage = () => {
    imageRow.replaceChildren(imageControl(picture, paintImage));
    picture.name = title.value.trim();
  };
  paintImage();
  imageField.append(imageRow, el('small', '', 'Shown across the top of the event\u2019s page and on its share card. Without one, the first category\u2019s picture stands in.'));
  imageField.hidden = !picture.image;
  const addPicture = el('button', 'link-button event-add-picture');
  addPicture.type = 'button';
  addPicture.append(svg('image'), el('span', '', 'Add a picture'));
  addPicture.hidden = Boolean(picture.image);
  addPicture.addEventListener('click', () => {
    imageField.hidden = false;
    addPicture.hidden = true;
  });
  eventPanel.append(addPicture, imageField);

  // Who and what: the classroom chips and the categories, as the filters
  // have them.
  const rooms = new Set(from ? from.classrooms : classroomNames());
  const cats = new Set(from ? from.tags.filter(t => !classroomNames().includes(t)) : []);
  const chip = (label, on, onClick, color) => {
    const b = el('button', 'filter-chip' + (on ? ' is-on' : ''), label);
    b.type = 'button';
    if (color) {
      b.style.setProperty('--room', color);
      b.classList.add('has-color');
    }
    b.addEventListener('click', onClick);
    return b;
  };
  tagPanel.append(el('p', 'hint', 'Everyone at Helios can see any event, but you can specify who the event is for.'));
  const roomField = el('div', 'field');
  roomField.append(el('span', '', 'Classrooms'));
  const roomChips = el('div', 'filter-chips');
  const paintRooms = () => {
    roomChips.replaceChildren();
    roomChips.append(chip('Everyone', rooms.size === classroomNames().length, () => {
      for (const c of classroomNames()) {
        rooms.add(c);
      }
      paintRooms();
    }));
    for (const band of bands()) {
      for (const c of band.classrooms) {
        roomChips.append(chip(c.name, rooms.has(c.name), () => {
          if (rooms.has(c.name)) {
            rooms.delete(c.name);
          } else {
            rooms.add(c.name);
          }
          paintRooms();
        }, state.model.colors[c.name]));
      }
    }
  };
  paintRooms();
  roomField.append(roomChips, el('small', '', 'Who the event is for. Everyone is the whole school.'));
  tagPanel.append(roomField);
  const catField = el('div', 'field');
  catField.append(el('span', '', 'Categories'));
  const catLines = el('div', 'form-tag-groups');
  const paintCats = () => {
    catLines.replaceChildren();
    for (const group of tagGroups()) {
      const line = el('div', 'form-tag-group');
      line.append(el('span', 'form-tag-label', group.name || 'Other categories'));
      const chips = el('div', 'filter-chips');
      for (const t of group.tags.filter(t => !t.builtIn)) {
        chips.append(chip(t.name, cats.has(t.name), () => {
          if (cats.has(t.name)) {
            cats.delete(t.name);
          } else {
            cats.add(t.name);
          }
          paintCats();
        }));
      }
      line.append(chips);
      catLines.append(line);
    }
  };
  paintCats();
  catField.append(catLines, el('small', '', 'What kind of thing it is. An event with none is filed under Misc.'));
  tagPanel.append(catField);

  const keywords = text(from ? (from.keywords || []).join(', ') : '', 'half day, kinder, short day');
  tagPanel.append(field('Search words', keywords, 'Words a parent might type that are not in the title, separated by commas.'));

  // Next, under the first tab, checks what it holds and turns to the
  // second, whose own button adds the event.
  const nextRow = el('div', 'modal-actions');
  const next = el('button', 'button');
  next.type = 'button';
  next.append(el('span', '', 'Next'), svg('chevron'));
  next.addEventListener('click', () => {
    if (!title.value.trim() || !startDate.value) {
      form.reportValidity();
      return;
    }
    showPanel(tagPanel);
  });
  nextRow.append(next);
  eventPanel.append(nextRow);
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg(edit ? 'check' : 'plus'), el('span', '', edit ? 'Save changes' : from ? 'Add the copy' : admin ? 'Add the event' : 'Share the event'));
  actions.append(submit, status);
  tagPanel.append(actions);
  form.addEventListener('submit', async e => {
    e.preventDefault();
    if (!rooms.size) {
      status.textContent = 'Pick at least one classroom.';
      status.classList.add('error');
      return;
    }
    const when = (date, time) => (date ? date + (time ? ' ' + time : '') : '');
    // An end with no time of its own ends when it starts, as the sheet
    // takes it; an all-day end is its day.
    const body = {
      title: title.value.trim(), start: when(startDate.value, startTime.value), end: when(endDate.value || startDate.value, endTime.value || startTime.value),
      location: place.value.trim(), description: description.value.trim(), source: source.value.trim(), image: picture.image, inviteOnly,
      tags: [...classroomNames().filter(c => rooms.has(c)), ...cats], keywords: keywords.value.split(',').map(w => w.trim()).filter(Boolean),
    };
    submit.disabled = true;
    status.classList.remove('error');
    status.textContent = edit ? 'Saving…' : 'Adding…';
    if (edit) {
      body.id = edit.id;
    } else if (slug && slug.value.trim()) {
      body.id = slug.value.trim();
    }
    const res = await fetch('/api/calendar/events', {method: edit ? 'PUT' : 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
    submit.disabled = false;
    if (!res.ok) {
      status.textContent = await res.text();
      status.classList.add('error');
      return;
    }
    status.textContent = '';
    if (edit) {
      toast('Saved');
      await onDone([edit.id]);
      return;
    }
    const {ids, pending} = await res.json();
    toast(pending ? 'Shared - an admin will approve it onto the calendar. It is on yours now.' : inviteOnly ? 'Added - send people the link from the event\u2019s page.' : 'Event added', pending || inviteOnly ? 6000 : 2600);
    await onDone(ids);
  });
  return form;
}

