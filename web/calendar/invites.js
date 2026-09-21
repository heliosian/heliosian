import {state, me, answer, isParty, eventDates, weekdayLong, parseDate, timeLine} from './state.js';
import {el, svg, button, toast, avatar, popup, copyText, segmented} from './dom.js';
import {appOrigin} from '/toolbar.js';
import {rulesEditor, filterWidgets} from '/rules.js';
import {createPersonPicker} from '/picker.js';
import {imageControl} from './images.js';

// The guest list of an event: built by its hosts from the directory, the
// classrooms, their own lists in Who?, a party's ticket holders and
// addresses from outside, then sent - two steps, so a list is finished
// before anyone hears of it. Whoever is invited answers for everyone in
// their household who is on the list and brings guests by name; a host
// reads every answer and corrects any. The server's side is
// internal/calendar/invites.go.

const answerWords = {yes: 'Yes', maybe: 'Maybe', no: 'No'};

// choice is a segmented bar that repaints itself as it is picked, handing
// each pick on.
function choice(items, active, onPick) {
  const wrap = el('div', 'choice');
  const paint = () => {
    wrap.replaceChildren(segmented(items, active, key => {
      active = key;
      paint();
      onPick(key);
    }));
  };
  paint();
  return wrap;
}

async function post(method, path, body) {
  const res = await fetch(path, {method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.status === 204 ? null : res.json();
}

// startParty starts a party's guest list from Celebrate's Create Invite:
// the invitation, and a group for its ticket holders with Auto-invite on.
export function startParty(e) {
  return post('POST', '/api/calendar/invites/start', {id: e.id});
}

// fetchInvites is the guest list as the viewer may see it, or null when
// the event has none to show them.
export async function fetchInvites(e) {
  try {
    const res = await fetch('/api/calendar/invites?id=' + encodeURIComponent(e.id));
    if (!res.ok) {
      return null;
    }
    return await res.json();
  } catch (err) {
    return null;
  }
}

// firstName is how a household member is spoken to: the first word of
// their name.
function firstName(p) {
  return (p.name || p.email || '').split(' ')[0];
}

// answerButtons is Yes, Maybe and No for one person, the one given lit,
// each click sending the word - or clearing it when it is lit already.
function answerButtons(row, e, onChange, {small = true} = {}) {
  const wrap = el('div', 'rsvp-mini' + (small ? ' is-small' : ''));
  const paint = () => {
    wrap.replaceChildren();
    for (const [word, label] of Object.entries(answerWords)) {
      const b = el('button', 'rsvp-mini-choice rsvp-mini-' + word + (row.answer === word ? ' is-on' : ''));
      b.type = 'button';
      b.append(svg(word === 'yes' ? 'check' : word === 'no' ? 'close' : 'clock'), el('span', '', label));
      b.disabled = !row.mine;
      b.addEventListener('click', async () => {
        const next = row.answer === word ? '' : word;
        try {
          if (row.key === me().email) {
            await answer(e, next);
          } else {
            await post('POST', '/api/calendar/invites/answer', {id: e.id, email: row.key, answer: next});
          }
          row.answer = next;
          paint();
          onChange(row, next);
        } catch (err) {
          toast(err.message);
        }
      });
      wrap.append(b);
    }
  };
  paint();
  return wrap;
}

// face is a person's photo or initial with, for a student, their grade
// badged on its corner in Who?'s colour.
function face(p, className) {
  const node = avatar(p, className || 'invite-face');
  if (p.grade) {
    const badge = el('span', 'grade-badge', /^kindergarten$/i.test(p.grade) ? 'K' : p.grade.replace(/^grade\s*/i, ''));
    badge.title = p.grade;
    const color = (state.model.gradeColors || {})[p.grade];
    if (color) {
      badge.style.background = `color-mix(in srgb, ${color} 65%, black)`;
    }
    node.append(badge);
  }
  return node;
}

// guestForm adds a guest for someone on the list: from the directory,
// through the person picker every app shares, or from outside by name
// and address; put down as a yes to start, and sent their own invitation
// when they have somewhere to send it - each a box to untick.
function guestForm(e, of, onDone) {
  const form = el('form', 'admin-form guest-form');
  const tabs = el('div', 'tabs');
  const panels = {};
  let active = 'helios';
  for (const [key, label] of [['helios', 'From Helios'], ['outside', 'Someone else']]) {
    const b = el('button', 'tab-button' + (key === active ? ' is-active' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      active = key;
      for (const t of tabs.children) {
        t.classList.toggle('is-active', t === b);
      }
      for (const [k, panel] of Object.entries(panels)) {
        panel.hidden = k !== key;
      }
    });
    tabs.append(b);
  }
  form.append(tabs);
  const field = (label, input) => {
    const wrap = el('label', 'field');
    wrap.append(el('span', '', label), input);
    return wrap;
  };
  // From Helios: the picker over everyone in the directory.
  const helios = el('div');
  const mount = el('div', 'cohost-picker');
  const picker = createPersonPicker(mount);
  fetchPickerData(e).then(data => {
    picker.setPeople(data.people.map(p => ({name: p.name || p.email, email: p.email})));
    if (active === 'helios') {
      mount.querySelector('input')?.focus();
    }
  }).catch(err => toast(err.message));
  helios.append(field('Who', mount));
  panels.helios = helios;
  // Someone else: a name, and an address if they have one.
  const outside = el('div');
  outside.hidden = true;
  const name = el('input');
  name.type = 'text';
  name.maxLength = 200;
  name.placeholder = 'Full name';
  const email = el('input');
  email.type = 'email';
  email.placeholder = 'Optional';
  outside.append(field('Name', name), field('Email', email));
  panels.outside = outside;
  form.append(helios, outside);
  // The choices: their answer, and their own invitation.
  const choices = el('div', 'guest-form-choices');
  const yes = el('input');
  yes.type = 'checkbox';
  yes.checked = true;
  const yesLabel = el('label', 'message-to-choice');
  yesLabel.append(yes, el('span', '', 'RSVP them as Yes'));
  const invite = el('input');
  invite.type = 'checkbox';
  invite.checked = true;
  const inviteLabel = el('label', 'message-to-choice');
  inviteLabel.append(invite, el('span', '', 'Send them their own invitation'));
  choices.append(yesLabel, inviteLabel);
  form.append(choices);
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('plus'), el('span', '', 'Add guest'));
  actions.append(submit, status);
  form.append(actions);
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    const body = {id: e.id, of, answer: yes.checked ? 'yes' : '', invite: invite.checked};
    if (active === 'helios') {
      if (!picker.value) {
        status.textContent = 'Pick someone from the directory first.';
        status.classList.add('error');
        return;
      }
      body.email = picker.value;
      body.name = picker.text || picker.value;
    } else {
      if (!name.value.trim()) {
        status.textContent = 'A name, please.';
        status.classList.add('error');
        return;
      }
      if (invite.checked && !email.value.trim()) {
        status.textContent = 'An email address is needed to send them an invitation - or untick that.';
        status.classList.add('error');
        return;
      }
      body.name = name.value.trim();
      body.email = email.value.trim();
    }
    submit.disabled = true;
    try {
      await post('POST', '/api/calendar/invites/guest', body);
      toast(`${body.name} added`);
      onDone();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      submit.disabled = false;
    }
  });
  return form;
}

function openGuestForm(e, of, onDone) {
  let shut = null;
  const form = guestForm(e, of, () => {
    shut();
    onDone();
  });
  shut = popup('Add a guest', form).shut;
}

// familyBand is the ask for someone invited, laid out as HCA-Team lays
// out My Family's Roles: RSVP Requested over the swoosh, who by, then a
// list card with a row for everyone in the household who is on the list -
// face, name, their line, and at the right Yes, Maybe and No until
// answered, then "You responded Yes" with Edit - the guests they have
// brought among them, and Add a guest and Hide event under the card.
export function familyBand(e, view, refresh) {
  const box = el('div', 'fam-box invite-box');
  box.append(el('h2', 'section section-swoosh', 'RSVP Requested'));
  const hosts = view.hosts.filter(h => h.email !== me().email).map(h => h.name).filter(Boolean);
  box.append(el('p', 'invite-by', (hosts.length ? `Invited by ${hosts.join(' and ')}. ` : '') + (view.mine.length > 1 ? 'Who\u2019s coming from your family?' : 'Are you going?')));
  const list = el('div', 'fam-list');
  for (const r of view.mine) {
    const row = el('div', 'fam-row invite-row' + (r.guestOf ? ' is-guest' : ''));
    row.append(face(r, 'avatar'));
    const text = el('div', 'fam-text');
    text.append(el('div', 'fam-name', r.key === me().email ? 'You' : r.name));
    const note = r.guestOf ? `Guest of ${r.guestOfName}` : r.line;
    if (note) {
      text.append(el('div', 'fam-where', note));
    }
    row.append(text);
    // Answered, the row says so with Edit; until then, or once Edit is
    // pressed, Yes, Maybe and No.
    const tools = el('div', 'invite-tools');
    const paintTools = editing => {
      tools.replaceChildren();
      if (r.answer && !editing) {
        // A filled mark in the answer's colour - a tick, a clock, a cross
        // - before "You said Yes".
        const said = el('span', 'invite-said is-' + r.answer);
        const mark = el('span', 'invite-said-mark');
        mark.append(svg(r.answer === 'yes' ? 'check' : r.answer === 'maybe' ? 'clock' : 'close'));
        said.append(mark, el('span', '', `${r.key === me().email ? 'You' : firstName(r)} said `), el('strong', '', answerWords[r.answer]));
        tools.append(said);
        if (r.mine) {
          tools.append(button('Edit', 'pencil', 'link-button fam-edit', () => paintTools(true)));
        }
        return;
      }
      tools.append(answerButtons(r, e, () => {
        refresh();
      }));
    };
    paintTools(false);
    if (r.guestOf && r.mine) {
      const remove = el('button', 'invite-remove');
      remove.type = 'button';
      remove.title = 'Remove this guest';
      remove.append(svg('close'));
      remove.addEventListener('click', async () => {
        try {
          await post('DELETE', '/api/calendar/invites/people', {id: e.id, email: r.key});
          toast(`${r.name} removed`);
          refresh();
        } catch (err) {
          toast(err.message);
        }
      });
      tools.append(remove);
    }
    row.append(tools);
    list.append(row);
  }
  box.append(list);
  const foot = el('div', 'invite-foot');
  // A guest comes with whoever in the household the viewer may answer
  // for - the viewer themselves when they are on the list.
  const of = view.mine.find(r => r.key === me().email && !r.guestOf) ? me().email : (view.mine.find(r => r.mine && !r.guestOf) || {}).key;
  if (of) {
    foot.append(button('Add a guest', 'plus', 'link-button', () => openGuestForm(e, of, refresh)));
  }
  const hide = el('button', 'rsvp-clear', 'Hide event');
  hide.type = 'button';
  hide.addEventListener('click', async () => {
    try {
      await answer(e, 'hidden');
      toast('Hidden - it shows on the month in gray');
      refresh();
    } catch (err) {
      toast(err.message);
    }
  });
  foot.append(hide);
  box.append(foot);
  return box;
}

// familyAnswered says whether everyone the viewer answers for has
// answered - when the ask leaves the page's head, My RSVP in the rail
// carrying the answers.
export function familyAnswered(view) {
  const mine = view.mine.filter(r => r.mine);
  return mine.length > 0 && mine.every(r => r.answer);
}

// rsvpRow is My RSVP, compact, as a row of the rail's card: each of the
// household on the list as a filled mark in their answer's colour - a
// tick, a clock, a cross, a question mark for no answer yet - and their
// name, which the viewer clicks to open Yes, Maybe and No in place for
// anyone they answer for; the guests they brought with a cross, then Add
// a guest and Hide event.
export function rsvpRow(e, view, refresh) {
  const row = el('div', 'side-row rsvp-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('calcheck'));
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', 'My RSVP'));
  for (const r of view.mine) {
    const line = el('div', 'rsvp-row-line' + (r.guestOf ? ' is-guest' : ''));
    const paintLine = () => {
      line.replaceChildren();
      const said = el(r.mine ? 'button' : 'span', 'invite-said rsvp-row-said is-' + (r.answer || 'none'));
      if (r.mine) {
        said.type = 'button';
        said.title = 'Change the answer';
      }
      const mark = el('span', 'invite-said-mark');
      mark.append(svg(r.answer === 'yes' ? 'check' : r.answer === 'maybe' ? 'clock' : r.answer === 'no' ? 'close' : 'info'));
      said.append(mark, el('strong', '', r.key === me().email ? 'You' : firstName(r)), el('span', 'rsvp-row-word', r.answer ? answerWords[r.answer] : 'No response'));
      if (r.mine) {
        said.addEventListener('click', () => {
          line.replaceChildren(el('span', 'rsvp-row-name', r.key === me().email ? 'You' : firstName(r)), answerButtons(r, e, () => {
            paintLine();
            refresh();
          }));
        });
      }
      line.append(said);
      if (r.guestOf && r.mine) {
        const remove = el('button', 'invite-remove rsvp-row-remove');
        remove.type = 'button';
        remove.title = 'Remove this guest';
        remove.append(svg('close'));
        remove.addEventListener('click', async () => {
          try {
            await post('DELETE', '/api/calendar/invites/people', {id: e.id, email: r.key});
            toast(`${r.name} removed`);
            refresh();
          } catch (err) {
            toast(err.message);
          }
        });
        line.append(remove);
      }
    };
    paintLine();
    body.append(line);
  }
  const foot = el('div', 'invite-foot');
  const of = view.mine.find(r => r.key === me().email && !r.guestOf) ? me().email : (view.mine.find(r => r.mine && !r.guestOf) || {}).key;
  if (of) {
    foot.append(button('Add a guest', 'plus', 'link-button', () => openGuestForm(e, of, refresh)));
  }
  const hide = el('button', 'rsvp-clear', 'Hide event');
  hide.type = 'button';
  hide.addEventListener('click', async () => {
    try {
      await answer(e, 'hidden');
      toast('Hidden - it shows on the month in gray');
      refresh();
    } catch (err) {
      toast(err.message);
    }
  });
  foot.append(hide);
  body.append(foot);
  row.append(icon, body);
  return row;
}

// comingCard is who is coming, for everyone invited while the hosts keep
// the list open, and for the hosts always: the yeses and the maybes as
// face tiles, a guest under whoever brought them.
export function comingCard(e, view, refresh) {
  const card = el('section', 'rsvps-card coming-section');
  // For a host, the invites still to go out lead the card, with the way
  // to send them.
  if (view.host) {
    const unsent = view.list.filter(r => r.invited && !r.sent && r.email);
    if (unsent.length) {
      const pending = el('div', 'rsvps-pending');
      pending.append(el('div', 'rsvps-pending-title', `Pending \u00b7 ${unsent.length} not sent yet`));
      const row = el('div', 'rsvps-pending-row');
      row.append(button('Send invitation now', 'calendar', 'button button-small', () => sendInvites(e, 'new', `Send the invitation to ${unsent.length} ${unsent.length === 1 ? 'person' : 'people'} who have not had it yet? Each gets an email with the calendar invite; a student's goes to them and their parents.`, refresh)));
      row.append(button('More info', 'info', 'link-button', () => openPending(e, view, refresh)));
      // Before anyone has been sent the invitation, Delete takes it all
      // back - the guest list, and a hand-added event with it.
      if (!view.sent) {
        row.append(button(view.party ? 'Delete invite' : 'Delete event', 'trash', 'link-button rsvps-pending-delete', () => deleteInvitation(e, view)));
      }
      pending.append(row);
      card.append(pending);
    }
  }
  // Everyone reads the yeses, the maybes and who has not answered; the
  // nos are the hosts' alone.
  const rows = (view.host ? view.list : view.coming).filter(r => r.answer === 'yes' || r.answer === 'maybe' || (view.host && r.answer === 'no') || (r.invited && !r.answer));
  const head = el('div', 'rsvps-card-head');
  head.append(el('h2', 'section section-swoosh', `Who\u2019s coming${rows.length ? ` (${rows.length})` : ''}`));
  card.append(head);
  // The filters every list has - search, the kinds of people, RSVP, grade,
  // classroom - over the faces, which repaint as they change.
  const grid = el('div', 'coming-grid');
  const paint = shown => {
    grid.replaceChildren();
    const groups = [
      ['Yes', shown.filter(r => r.answer === 'yes'), 'is-yes'],
      ['Maybe', shown.filter(r => r.answer === 'maybe'), 'is-maybe'],
    ];
    if (view.host) {
      groups.push(['No', shown.filter(r => r.answer === 'no'), 'is-no']);
    }
    groups.push(['No response', shown.filter(r => r.invited && !r.answer), 'is-waiting']);
    if (!groups.some(([, people]) => people.length)) {
      grid.append(el('div', 'side-line', rows.length ? 'Nobody matches.' : 'Nobody has answered yet.'));
    }
    for (const [label, people, cls] of groups) {
      if (!people.length) {
        continue;
      }
      grid.append(el('div', 'rsvps-head ' + cls, `${label} \u00b7 ${people.length}`));
      // The faces as Celebrate lays out its Who's Coming: a card each, the
      // photo square across its width, the name, the line that places
      // them, and Your family on the household's own.
      const list = el('div', 'attendee-grid');
      for (const p of people) {
        const tile = el('button', 'attendee');
        tile.type = 'button';
        tile.title = p.name || p.email;
        const photo = face(p, 'attendee-face');
        // On a party, a host sees a mark at the face's corner: a ticket
        // for one bought, a gift for one the hosts gave, an hourglass for
        // the waitlist, an empty ring for none.
        if (view.party && view.host && p.invited) {
          const mark = el('span', 'ticket-mark is-' + (p.ticket || 'none'));
          mark.title = ticketWords(p.ticket);
          mark.append(svg(p.ticket === 'ticket' ? 'ticket' : p.ticket === 'free' ? 'gift' : p.ticket === 'waitlist' ? 'clock' : 'close'));
          photo.append(mark);
        }
        tile.append(photo, el('div', 'attendee-name', p.name || p.email));
        const line = p.guestOf ? `Guest of ${p.guestOfName}` : p.line;
        if (line) {
          tile.append(el('div', 'attendee-line', line));
        }
        if (view.mine.some(m => m.key === p.key)) {
          tile.append(el('div', 'attendee-mine', 'Your family'));
        }
        tile.addEventListener('click', () => openGuestCard(e, p, view, refresh));
        list.append(tile);
      }
      grid.append(list);
    }
  };
  if (rows.length > 1) {
    card.append(listFilters(rows, view, paint, {rsvp: false}));
  }
  paint(rows);
  card.append(grid);
  // For a host, who may read the card, with the way to change it.
  if (view.host) {
    const visible = el('div', 'rsvps-visible');
    const open = view.guestList !== 'private';
    visible.append(el('span', '', open ? 'Guest list is visible to Helios guests.' : 'Guest list is visible to hosts only.'));
    visible.append(button('change', null, 'rsvp-clear', () => openVisibility(e, view, refresh)));
    card.append(visible);
  }
  return card;
}

// openVisibility is the choice of who may read who is coming, in a
// popup: Helios guests - everyone invited - or the hosts only.
function openVisibility(e, view, refresh) {
  const form = el('form', 'admin-form');
  form.append(el('p', 'hint', 'Who may see who is coming - the names under Yes, Maybe and No response on the event\u2019s page. The hosts always see everyone, the nos included.'));
  let guestList = view.guestList === 'private' ? 'private' : 'public';
  form.append(choice([{key: 'public', label: 'Helios guests'}, {key: 'private', label: 'Hosts only'}], guestList, key => {
    guestList = key;
  }));
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  actions.append(submit, status);
  form.append(actions);
  let shut = null;
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    submit.disabled = true;
    try {
      await post('PUT', '/api/calendar/invites/settings', {id: e.id, guestList});
      toast(guestList === 'private' ? 'Now hosts only' : 'Now visible to Helios guests');
      shut();
      refresh();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      submit.disabled = false;
    }
  });
  shut = popup('Who can see who is coming', form).shut;
}

// openMessage is a host's message to the list: who it goes to by where
// they stand - tiles for Yes, Maybe, No and No response with their counts,
// and Pick & Choose for particular people - the subject behind the
// event's name, the words, and Send. A preset starts it as an RSVP
// reminder or an event reminder, the recipients, subject and words filled
// in to change, with the calendar invite attached under the hood.
function openMessage(e, view, refresh, preset = {}) {
  const box = el('div', 'compose');
  // The head: a mark, the title, and a line on what happens.
  const head = el('div', 'compose-head');
  const mark = el('div', 'compose-mark');
  mark.append(svg('chat'));
  const words = el('div');
  words.append(el('h2', 'compose-title', preset.title || 'Send a message'), el('p', 'compose-lead', 'Invites and messages to students are sent to their parents too.'));
  head.append(mark, words);
  box.append(head);
  // Send to: the tiles, and Pick & Choose.
  const counts = view.counts || {};
  const to = new Set(preset.to || ['yes', 'maybe', 'none']);
  const picked = new Set();
  box.append(el('div', 'compose-label', 'Send to'));
  const tiles = el('div', 'send-tiles');
  const tile = (key, label, n, cls) => {
    const t = el('label', 'send-tile ' + cls + (to.has(key) ? ' is-on' : ''));
    const cb = el('input');
    cb.type = 'checkbox';
    cb.checked = to.has(key);
    const text = el('span', 'send-tile-words');
    text.append(el('span', 'send-tile-label', label), el('span', 'send-tile-note', String(n)));
    t.append(cb, text);
    cb.addEventListener('change', () => {
      t.classList.toggle('is-on', cb.checked);
      if (cb.checked) {
        to.add(key);
      } else {
        to.delete(key);
      }
    });
    return t;
  };
  // Pick & Choose opens the guest list in a popup of its own to tick
  // particular people; the tile says how many are picked.
  const pickTile = el('button', 'send-tile is-pick');
  pickTile.type = 'button';
  const pickWords = el('span', 'send-tile-words');
  const pickNote = el('span', 'send-tile-note', 'Select specific people');
  pickWords.append(el('span', 'send-tile-label', 'Pick & Choose'), pickNote);
  const pickIcon = el('span', 'send-tile-icon');
  pickIcon.append(svg('people'));
  pickTile.append(pickIcon, pickWords);
  const paintPick = () => {
    pickTile.classList.toggle('is-on', picked.size > 0);
    pickNote.textContent = picked.size ? `${picked.size} picked` : 'Select specific people';
  };
  pickTile.addEventListener('click', () => openPick(e, view, picked, paintPick));
  tiles.append(
    tile('yes', 'Yes', counts.yes || 0, 'is-yes'),
    tile('maybe', 'Maybe', counts.maybe || 0, 'is-maybe'),
    tile('no', 'No', counts.no || 0, 'is-no'),
    tile('none', 'No response', counts.waiting || 0, 'is-waiting'),
    pickTile,
  );
  box.append(tiles);
  // The subject, the event's name before it in brackets.
  box.append(el('div', 'compose-label', 'Subject'));
  const subject = el('input');
  subject.type = 'text';
  subject.required = true;
  subject.maxLength = 200;
  subject.placeholder = 'What to bring, a change of plan, a reminder\u2026';
  subject.value = preset.subject || '';
  const subjectRow = el('div', 'message-subject');
  subjectRow.append(el('span', 'message-subject-prefix', `[${e.title}]`), subject);
  box.append(subjectRow);
  // The words, with how many of the thousand are used.
  box.append(el('div', 'compose-label', 'Message'));
  const message = el('textarea', 'compose-message');
  message.rows = 6;
  message.maxLength = 1000;
  message.value = preset.message || '';
  const counter = el('div', 'compose-count');
  const paintCount = () => {
    counter.textContent = `${message.value.length}/1000`;
  };
  message.addEventListener('input', paintCount);
  paintCount();
  const messageWrap = el('div', 'compose-message-wrap');
  messageWrap.append(message, counter);
  box.append(messageWrap);
  // Cancel and Send.
  const actions = el('div', 'compose-actions');
  const status = el('span', 'save-status');
  let shut = null;
  const cancel = button('Cancel', null, 'button button-secondary', () => shut());
  const send = button('Send message', 'chat', 'button', async () => {
    if (!to.size && !picked.size) {
      status.textContent = 'Pick who to send to.';
      status.classList.add('error');
      return;
    }
    if (!subject.value.trim() || !message.value.trim()) {
      status.textContent = 'A subject and a message, please.';
      status.classList.add('error');
      return;
    }
    send.disabled = true;
    try {
      const made = await post('POST', '/api/calendar/invites/message', {id: e.id, subject: subject.value.trim(), message: message.value.trim(), to: [...to], emails: [...picked], attach: Boolean(preset.attach)});
      toast(made.messages === 1 ? 'Sent to one person' : `Sent to ${made.messages} people`, 4000);
      shut();
      refresh();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      send.disabled = false;
    }
  });
  actions.append(status, cancel, send);
  box.append(actions);
  shut = popup('', box, {wide: true}).shut;
  setTimeout(() => (preset.subject ? message : subject).focus(), 0);
}

// hostsRow is the event's hosts as a row of the rail's card - their
// names in a line, then each a face and a name to
// their page on Who?, and for a host the way to add a co-host - the
// person picker every app shares over the directory's adults - and to
// take a co-host off (never the event's own host, nor a party's hosts on
// Celebrate, who are hosts by the other app's word). A co-host builds and
// sends the list and reads every answer, as the host does.
export function hostsRow(e, view, refresh) {
  const row = el('div', 'side-row hosts-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('people'));
  const card = el('div', 'side-row-body');
  card.append(el('div', 'side-title', 'Hosts'));
  const names = view.hosts.map(h => h.name || h.email).filter(Boolean);
  if (names.length) {
    card.append(el('div', 'side-line', names.length > 1 ? names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1] : names[0]));
  }
  const s = view.settings || {};
  const cohosts = new Set(s.hosts || []);
  const list = el('div', 'rsvps-grid');
  for (const h of view.hosts) {
    const tile = el(h.email ? 'a' : 'div', 'contact-card');
    if (h.email) {
      tile.href = appOrigin('who') + '/people/' + encodeURIComponent(h.email);
    }
    tile.title = [h.name, h.line].filter(Boolean).join(' \u00b7 ');
    tile.append(face(h, 'contact-photo'), el('span', 'contact-name', h.name || h.email));
    if (view.host && cohosts.has(h.email)) {
      const x = el('button', 'hosts-card-remove');
      x.type = 'button';
      x.title = `Take ${h.name} off as a co-host`;
      x.textContent = '\u00d7';
      x.addEventListener('click', async ev => {
        ev.preventDefault();
        if (!confirm(`Take ${h.name} off as a co-host?`)) {
          return;
        }
        try {
          await post('PUT', '/api/calendar/invites/settings', {id: e.id, hosts: [...cohosts].filter(x => x !== h.email)});
          refresh();
        } catch (err) {
          toast(err.message);
        }
      });
      tile.append(x);
    }
    list.append(tile);
  }
  card.append(list);
  if (view.host) {
    card.append(button('Add co-host', 'plus', 'link-button', () => openAddHost(e, view, refresh)));
  }
  row.append(icon, card);
  return row;
}

// openAddHost is the small popup that adds a co-host: the person picker
// over the directory's adults, and Add.
function openAddHost(e, view, refresh) {
  const form = el('form', 'admin-form');
  form.append(el('p', 'hint', 'A co-host builds and sends the list, reads every answer and hears replies, as you do.'));
  const mount = el('div', 'cohost-picker');
  const picker = createPersonPicker(mount);
  fetchPickerData(e).then(data => {
    picker.setPeople(data.people.filter(p => !p.isStudent && !view.hosts.some(h => h.email === p.email)).map(p => ({name: p.name || p.email, email: p.email})));
    mount.querySelector('input')?.focus();
  }).catch(err => toast(err.message));
  form.append(mount);
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('plus'), el('span', '', 'Add co-host'));
  actions.append(submit, status);
  form.append(actions);
  let shut = null;
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    const email = picker.value;
    if (!email) {
      status.textContent = 'Pick someone from the directory first.';
      status.classList.add('error');
      return;
    }
    submit.disabled = true;
    try {
      await post('PUT', '/api/calendar/invites/settings', {id: e.id, hosts: [...((view.settings || {}).hosts || []), email]});
      toast('Co-host added');
      shut();
      refresh();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      submit.disabled = false;
    }
  });
  shut = popup('Add a co-host', form).shut;
}

// menuButton is a button that drops a small menu of choices, as the
// banner's image button does.
function menuButton(label, icon, items) {
  const holder = el('div', 'hero-image-menu-holder');
  const toggle = el('button', 'button button-secondary button-small');
  toggle.type = 'button';
  toggle.setAttribute('aria-haspopup', 'menu');
  toggle.append(svg(icon), el('span', '', label), svg('down'));
  const menu = el('div', 'hero-image-menu');
  menu.hidden = true;
  for (const item of items) {
    const b = el('button', 'hero-image-menu-item');
    b.type = 'button';
    b.append(svg(item.icon), el('span', '', item.words));
    b.addEventListener('click', () => {
      menu.hidden = true;
      item.run();
    });
    menu.append(b);
  }
  toggle.addEventListener('click', ev => {
    ev.stopPropagation();
    menu.hidden = !menu.hidden;
    if (!menu.hidden) {
      document.addEventListener('click', () => {
        menu.hidden = true;
      }, {once: true});
    }
  });
  menu.addEventListener('click', ev => ev.stopPropagation());
  holder.append(toggle, menu);
  return holder;
}

// ticketWords is a party's word on someone, short: bought, given, waiting,
// or none.
function ticketWords(ticket) {
  return ticket === 'ticket' ? 'Purchased ticket' : ticket === 'free' ? 'Free ticket' : ticket === 'waitlist' ? 'On the waitlist' : 'No ticket';
}

// ticketDetail is the same at length, for their card.
function ticketDetail(ticket) {
  return ticket === 'ticket' ? 'Their household bought a ticket on Helios Celebrate.'
    : ticket === 'free' ? 'A ticket the hosts gave at no charge.'
      : ticket === 'waitlist' ? 'Their household asked for a ticket; the party was full. They hold no ticket yet.'
        : 'Invited, with no ticket to the party. Tickets are on the party page on Helios Celebrate.';
}

// warningChip is the word of caution on an address that may not reach
// anyone - bounced, or a school address the directory does not hold -
// with Edit email beside it, which opens the address to change; the
// host may send to it as it stands all the same.
function warningChip(e, r, refresh) {
  const chip = el('span', 'guests-chip is-warning');
  chip.title = r.warningWords;
  chip.append(svg('info'), el('span', '', r.warning === 'bounced' ? 'Bounced' : 'Not in the directory'));
  const edit = el('button', 'guests-chip-edit');
  edit.type = 'button';
  edit.textContent = 'Edit email';
  edit.addEventListener('click', ev => {
    ev.stopPropagation();
    openEmailEdit(e, r, refresh);
  });
  chip.append(edit);
  return chip;
}

// openEmailEdit is the address to change, in a popup: the caution's
// words, the address as it stands, and Save.
function openEmailEdit(e, r, refresh) {
  const form = el('form', 'admin-form');
  form.append(el('p', 'hint', r.warningWords + ' Give a different address, or close this and send to it anyway.'));
  const field = el('div', 'field');
  const input = el('input', 'email-edit-input');
  input.type = 'email';
  input.required = true;
  input.value = r.email;
  field.append(el('span', '', 'Email address'), input);
  form.append(field);
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  actions.append(submit, status);
  form.append(actions);
  let shut = null;
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    submit.disabled = true;
    try {
      await post('POST', '/api/calendar/invites/email', {id: e.id, email: r.key, to: input.value.trim()});
      toast('Address changed');
      shut();
      refresh();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      submit.disabled = false;
    }
  });
  shut = popup(`${r.name || r.email}\u2019s email`, form).shut;
  setTimeout(() => input.select(), 0);
}

// openPending lists, for a host, everyone still to be sent the
// invitation, each with Send now, and Send all at the foot.
function openPending(e, view, refresh) {
  const box = el('div');
  const unsent = view.list.filter(r => r.invited && !r.sent && r.email);
  box.append(el('p', 'hint', 'Not sent the invitation yet. Each gets an email with the calendar invite; a student\u2019s goes to them and their parents.'));
  const list = el('div', 'picker-results');
  let shut = null;
  const sendTo = async (emails, words) => {
    try {
      const made = await post('POST', '/api/calendar/invites/send', {id: e.id, emails});
      toast(made.messages === 1 ? 'One invite is on its way' : `${made.messages} invites are on their way`, 4000);
      shut();
      refresh();
    } catch (err) {
      toast(err.message);
    }
  };
  for (const r of unsent) {
    const row = el('div', 'picker-person pending-row');
    row.append(face(r));
    const who = el('div', 'invite-who');
    who.append(el('div', 'invite-name', r.name || r.email));
    const line = [r.guestOf ? `Guest of ${r.guestOfName}` : r.line, r.email].filter(Boolean).join(' \u00b7 ');
    if (line) {
      who.append(el('div', 'invite-line', line));
    }
    if (r.warning) {
      const marks = el('div', 'guests-marks');
      marks.append(warningChip(e, r, () => {
        shut();
        refresh();
      }));
      who.append(marks);
    }
    const tools = el('div', 'pending-tools');
    tools.append(button('Send now', 'calendar', 'button button-small', () => sendTo([r.key])));
    // Skip: marked sent without an email, so they leave Pending and stand
    // among those with no reply yet.
    tools.append(button('Skip sending', null, 'link-button pending-skip', async () => {
      const name = r.name || r.email;
      if (!confirm(`${name} will be moved to No reply yet without being sent the invitation. Skip sending to ${name}?`)) {
        return;
      }
      try {
        await post('POST', '/api/calendar/invites/skip', {id: e.id, emails: [r.key]});
        toast(`${name} moved to No reply yet - no email sent`);
        shut();
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
    row.append(who, tools);
    list.append(row);
  }
  box.append(list);
  const actions = el('div', 'modal-actions');
  actions.append(button(`Send all ${unsent.length}`, 'calendar', 'button', () => sendTo(unsent.map(r => r.key))));
  box.append(actions);
  shut = popup(`Pending \u00b7 ${unsent.length} not sent yet`, box).shut;
}

// flyerCard is the invitation's flyer in the rail, as HCA-Team keeps an
// event's: the whole picture at the rail's width, opened full size on a
// click, and for a host Replace and Remove - a finished poster, uploaded
// whole, never found in a library; the first one is added from Edit the
// invitation. It is the picture a link to the event previews with.
export function flyerCard(e, view, refresh) {
  if (!view.flyer) {
    return null;
  }
  const card = el('div', 'side-card flyer-card');
  card.append(el('div', 'side-title', 'Flyer'));
  const open = el('a', 'flyer-open');
  open.href = view.flyer;
  open.target = '_blank';
  open.rel = 'noopener';
  open.title = 'View full size';
  const img = el('img', 'flyer-image');
  img.src = view.flyer;
  img.alt = `${e.title} flyer`;
  open.append(img);
  card.append(open);
  if (view.host) {
    const save = async image => {
      try {
        await post('PUT', '/api/calendar/invites/settings', {id: e.id, flyer: image});
        toast(image ? 'Flyer saved' : 'Flyer removed');
        refresh();
      } catch (err) {
        toast(err.message);
      }
    };
    const bar = el('div', 'flyer-actions');
    const file = el('input');
    file.type = 'file';
    file.accept = 'image/*';
    file.hidden = true;
    file.addEventListener('change', async () => {
      if (!file.files.length) {
        return;
      }
      try {
        const {uploadImage} = await import('./images.js');
        const made = await uploadImage(file.files[0]);
        await save(made.name);
      } catch (err) {
        toast(err.message);
      }
    });
    const upload = el('label', 'button button-secondary button-small');
    upload.append(svg('plus'), el('span', '', 'Replace'), file);
    bar.append(upload);
    if (view.flyer) {
      bar.append(button('Remove', 'trash', 'button button-secondary button-small', () => save('')));
    }
    card.append(bar);
  }
  return card;
}

// openGuestCard is one person on the list in a popup: for a host, their
// answer to correct - Yes, Maybe, No - and the way to take them off; for
// anyone else the small card - face, name, their line - and the way to
// their page on Helios Who?.
function openGuestCard(e, p, view, refresh) {
  const card = el('div', 'guest-card');
  const head = el('div', 'guest-card-head');
  head.append(face(p, 'contact-photo guest-card-face'));
  const who = el('div', 'invite-who');
  who.append(el('div', 'guest-card-name', p.name || p.email));
  const line = p.guestOf ? `Guest of ${p.guestOfName}` : p.line;
  if (line) {
    who.append(el('div', 'invite-line', line));
  }
  if (p.email && view.host) {
    who.append(el('div', 'invite-line', p.email));
  }
  // Their page on Who?, under their details.
  if (p.email && !p.outside) {
    const open = el('a', 'link-button guest-card-open');
    open.href = appOrigin('who') + '/people/' + encodeURIComponent(p.email);
    open.append(svg('open'), el('span', '', 'Open in Helios Who?'));
    who.append(open);
  }
  head.append(who);
  card.append(head);
  let shut = null;
  // On a party, a host reads their ticket standing plainly.
  if (view.party && view.host && p.invited) {
    const standing = el('div', 'guest-card-ticket is-' + (p.ticket || 'none'));
    standing.append(svg(p.ticket === 'ticket' ? 'ticket' : p.ticket === 'free' ? 'gift' : p.ticket === 'waitlist' ? 'clock' : 'close'));
    const words = el('div');
    words.append(el('strong', '', ticketWords(p.ticket)), el('div', 'invite-line', ticketDetail(p.ticket)));
    standing.append(words);
    card.append(standing);
  }
  if (view.host) {
    const ask = el('div', 'guest-card-ask');
    ask.append(el('div', 'rsvps-head', 'Their answer'));
    ask.append(answerButtons(p, e, () => {
      toast(p.answer ? `${firstName(p)}: ${answerWords[p.answer]}` : `${firstName(p)}\u2019s answer cleared`);
      refresh();
    }, {small: false}));
    if (p.answer) {
      ask.append(el('div', 'guests-by', answeredWords(p)));
    }
    card.append(ask);
  }
  const links = el('div', 'guest-card-links');
  // Resend: the invitation again, to them alone, once it has gone out -
  // Send now while it has not.
  if (view.host && p.invited && p.email) {
    links.append(button(p.sent ? 'Resend invitation' : 'Send invitation', 'calendar', 'link-button', async () => {
      try {
        const made = await post('POST', '/api/calendar/invites/send', {id: e.id, emails: [p.key]});
        toast(made.messages === 1 ? 'The invitation is on its way' : `${made.messages} emails are on their way`, 4000);
        shut();
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  if (view.host && p.invited) {
    links.append(button('Take off the list', 'trash', 'link-button danger', async () => {
      if (!confirm(`Take ${p.name || p.email} off the list? Their answer goes with them${p.via && p.via.startsWith('group:') ? ', and the group will not add them back' : ''}.`)) {
        return;
      }
      try {
        await post('DELETE', '/api/calendar/invites/people', {id: e.id, email: p.key});
        shut();
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  if (links.childElementCount) {
    card.append(links);
  }
  shut = popup(p.name || p.email, card).shut;
}

// openPick is Pick & Choose: everyone on the list with an address, each a
// row to tick, a search over them, and Done.
function openPick(e, view, picked, onDone) {
  const box = el('div');
  const search = el('input', 'picker-search');
  search.type = 'search';
  search.placeholder = 'Search the guest list';
  // Chips to keep to an answer - yes, maybe, no, no response - any
  // number lit, none meaning everyone.
  const kinds = new Set();
  const chips = el('div', 'filter-chips picker-roles');
  for (const [key, label] of [['yes', 'Yes'], ['maybe', 'Maybe'], ['no', 'No'], ['none', 'No response']]) {
    const chip = el('button', 'filter-chip pick-chip is-' + key, label);
    chip.type = 'button';
    chip.addEventListener('click', () => {
      if (kinds.has(key)) {
        kinds.delete(key);
      } else {
        kinds.add(key);
      }
      chip.classList.toggle('is-on', kinds.has(key));
      paintRows();
    });
    chips.append(chip);
  }
  const rows = el('div', 'picker-results send-pick-rows');
  const people = view.list.filter(r => r.invited && r.email);
  const paintRows = () => {
    rows.replaceChildren();
    const q = search.value.trim().toLowerCase();
    const shown = people.filter(r => (!q || (r.name || '').toLowerCase().includes(q) || r.email.toLowerCase().includes(q)) && (!kinds.size || kinds.has(r.answer || 'none')));
    if (!shown.length) {
      rows.append(el('div', 'picker-note', 'Nobody matches.'));
    }
    for (const r of shown) {
      const row = el('button', 'picker-person' + (picked.has(r.key) ? ' is-picked' : ''));
      row.type = 'button';
      const check = el('span', 'picker-check');
      check.append(svg('check'));
      row.append(check, face(r));
      const who = el('div', 'invite-who');
      who.append(el('div', 'invite-name', r.name || r.email));
      if (r.line) {
        who.append(el('div', 'invite-line', r.line));
      }
      row.append(who);
      // Their answer as a filled mark and a word at the row's end.
      const said = el('span', 'invite-said pick-said is-' + (r.answer || 'none'));
      const mark = el('span', 'invite-said-mark');
      mark.append(svg(r.answer === 'yes' ? 'check' : r.answer === 'maybe' ? 'clock' : r.answer === 'no' ? 'close' : 'info'));
      said.append(mark, el('strong', '', r.answer ? answerWords[r.answer] : 'No response'));
      row.append(said);
      row.addEventListener('click', () => {
        if (picked.has(r.key)) {
          picked.delete(r.key);
        } else {
          picked.add(r.key);
        }
        row.classList.toggle('is-picked', picked.has(r.key));
        paintDone();
      });
      rows.append(row);
    }
  };
  search.addEventListener('input', paintRows);
  paintRows();
  box.append(search, chips, rows);
  const actions = el('div', 'modal-actions');
  const done = el('button', 'button');
  done.type = 'button';
  const paintDone = () => {
    done.replaceChildren(svg('check'), el('span', '', picked.size ? `Done \u00b7 ${picked.size} picked` : 'Done'));
  };
  paintDone();
  let shut = null;
  done.addEventListener('click', () => {
    shut();
    onDone();
  });
  actions.append(done);
  box.append(actions);
  shut = popup('Pick & Choose', box, {wide: true}).shut;
  setTimeout(() => search.focus(), 0);
}

// sendInvites asks, then sends the invites - to everyone not yet sent
// one, or to everyone who has not answered - and redraws.
async function sendInvites(e, to, words, refresh) {
  if (!confirm(words)) {
    return;
  }
  try {
    const made = await post('POST', '/api/calendar/invites/send', {id: e.id, to});
    toast(made.messages === 1 ? 'One invite is on its way' : `${made.messages} invites are on their way`, 4000);
    refresh();
  } catch (err) {
    toast(err.message);
  }
}

// openGroups remembers which groups' people are unfolded on the list
// across redraws.
const openGroups = new Set();

const viaWords = {family: 'Family', search: 'Search', classroom: 'Classroom', list: 'List', tickets: 'Tickets', outside: 'By email', guest: 'Guest', link: 'By link'};

// viaLabel is how someone came to be on the list, in a word: the way a
// host found them, or the classroom or list by name; a host answering
// their own event is the host.
function viaLabel(r, view) {
  if (!r.invited) {
    return view.hosts.some(h => h.email === r.key) ? 'Host' : 'By link';
  }
  const [kind, rest] = (r.via || '').split(':');
  if (kind === 'group') {
    const g = (view.groups || []).find(x => x.id === rest);
    return g ? 'Group: ' + groupWords(g) : 'Group';
  }
  if (rest) {
    return rest;
  }
  return viaWords[kind] || '';
}

// stamp is a sheet timestamp as a short date.
function stamp(s) {
  const day = (s || '').slice(0, 10);
  const d = day ? new Date(day + 'T12:00:00') : null;
  return d && !isNaN(d) ? d.toLocaleDateString('en-US', {month: 'short', day: 'numeric'}) : '';
}

// moment is a sheet timestamp as a short date and time.
function moment(s) {
  const d = s && s.length >= 16 ? new Date(s.slice(0, 10) + 'T' + s.slice(11, 16) + ':00') : null;
  return d && !isNaN(d) ? d.toLocaleDateString('en-US', {month: 'short', day: 'numeric'}) + ' ' + d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'}) : stamp(s);
}

// answeredWords is how and when an answer came, for a host: by whom when
// not the person themselves, from their calendar app or a page, and the
// moment.
function answeredWords(r) {
  if (!r.answer) {
    return '';
  }
  const bits = [];
  if (r.answeredBy) {
    bits.push(`by ${r.answeredBy}`);
  }
  bits.push(r.answeredVia === 'calendar' ? 'from their calendar app' : 'on the page');
  if (r.answeredAt) {
    bits.push(moment(r.answeredAt));
  }
  return bits.join(' \u00b7 ');
}

// guestListSection is the hosts' side of an invitation, under the event:
// the counts, the tools - add people, settings, send - and every person on
// the list with their answer to correct, grouped by household.
export function guestListSection(e, view, refresh) {
  const section = el('section', 'side-card guests-section');
  const unsent = view.list.filter(r => r.invited && !r.sent).length;
  const waiting = view.list.filter(r => r.invited && !r.answer).length;
  // The head: the mark, the title with a word on where the list stands,
  // and Add people - with Send beside it once the invitation is out.
  const head = el('div', 'guests-head');
  const headMark = el('div', 'guests-head-mark');
  headMark.append(svg('groups'));
  const headWords = el('div', 'guests-head-words');
  headWords.append(el('div', 'guests-title', 'Guest list'));
  headWords.append(el('div', 'guests-subtitle', !view.sent
    ? (view.list.length ? 'Add everyone, then send invites.' : 'Nobody on the list yet.')
    : unsent ? `${unsent} added since the invites went out, not sent yet.` : 'Invites are out.'));
  head.append(headMark, headWords);
  const tools = el('div', 'guests-tools');
  tools.append(button('Add people', 'plus', 'button button-small', () => openPicker(e, view, refresh)));
  // Once the invitation is out: Send, a menu of a reminder to whoever has
  // not answered and a message to the list. The invites themselves go
  // from the Pending band in the rail.
  if (view.sent) {
    const first = eventDates(e)[0];
    const day = weekdayLong(first) + ', ' + parseDate(first).toLocaleDateString('en-US', {month: 'long', day: 'numeric'});
    const when = e.allDay ? day : `${day}, ${timeLine(e)}`;
    tools.append(menuButton('Send', 'calendar', [
      {icon: 'clock', words: `RSVP reminder \u00b7 ${waiting}`, run: () => openMessage(e, view, refresh, {
        title: 'Send RSVP reminder', to: ['none'], subject: 'Reminder: you\u2019re invited!', attach: true,
        message: `We haven\u2019t heard back from you yet - please let us know if you can make it on ${when}.`,
      })},
      {icon: 'calcheck', words: 'Event reminder', run: () => openMessage(e, view, refresh, {
        title: 'Send event reminder', to: ['yes', 'maybe', 'none'], subject: `Reminder: ${day}`, attach: true,
        message: `Just a reminder that we\u2019re on for ${when}${e.location ? ' at ' + e.location : ''}. See you there!`,
      })},
      {icon: 'chat', words: 'Message', run: () => openMessage(e, view, refresh)},
    ]));
  }
  head.append(tools);
  section.append(head);
  // The counts as tiles, two to a row - a mark, the number, the word -
  // each opening the Guest table kept to that answer (invited, to
  // everyone), and pending the Pending popup.
  const c = view.counts;
  const counts = el('div', 'guests-stats');
  const tableOf = answer => () => openTable(e, view, refresh, {answer});
  const stat = (n, label, icon, cls, onClick) => {
    const tile = el('button', 'guests-stat ' + cls);
    tile.type = 'button';
    tile.addEventListener('click', onClick);
    tile.append(svg(icon));
    const words = el('div', 'guests-stat-words');
    words.append(el('strong', '', String(n)), el('span', '', label));
    tile.append(words);
    return tile;
  };
  counts.append(
    stat(c.invited, 'invited', 'mail', 'is-all', tableOf('')),
    stat(c.yes, 'yes', 'check', 'is-yes', tableOf('yes')),
    stat(c.maybe, 'maybe', 'help', 'is-maybe', tableOf('maybe')),
    stat(c.no, 'no', 'ban', 'is-no', tableOf('no')),
    stat(c.waiting, 'no reply yet', 'reply', 'is-waiting', tableOf('none')),
  );
  const pending = view.list.filter(r => r.invited && !r.sent && r.email).length;
  if (pending) {
    counts.append(stat(pending, 'pending', 'clock', 'is-pending', () => openPending(e, view, refresh)));
  }
  // Once the invites are out, how many have opened theirs.
  if (view.sent) {
    counts.append(stat(view.list.filter(r => r.opened).length, 'opened', 'eye', 'is-opened', () => openTable(e, view, refresh, {opened: 'yes'})));
  }
  section.append(counts);
  if (!view.list.length) {
    section.append(el('p', 'guests-note', 'Add people from the directory, a classroom, one of your lists' + (view.party ? ', the ticket holders' : '') + ', or by email - then send the invitation.'));
  }
  // groupHead is a group's row at the head of its people: a chevron that
  // folds them, its sentence, how many it put on, Auto-invite to switch,
  // and a bin to take it off.
  const groupHead = (g, count, onToggle) => {
    const row = el('div', 'guests-group');
    const mark = el('div', 'guests-group-mark');
    mark.append(svg('groups'));
    row.append(mark);
    const who = el('div', 'invite-who');
    const line = `${count} on the list from this group` + (g.auto ? (g.sent ? ' \u00b7 auto-invited' : ' \u00b7 auto-invite starts after you send') : '');
    who.append(el('div', 'invite-name', groupWords(g)), el('div', 'invite-line', line));
    who.addEventListener('click', onToggle);
    row.append(who);
    const toggle = el('button', 'guests-group-toggle');
    toggle.type = 'button';
    toggle.title = 'Show or hide the people in this group';
    toggle.append(svg('down'));
    toggle.addEventListener('click', onToggle);
    row.append(toggle);
    const foot = el('div', 'guests-group-foot');
    const auto = el('label', 'guests-group-auto');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = g.auto;
    box.addEventListener('change', async () => {
      try {
        await post('PUT', '/api/calendar/invites/group', {id: e.id, group: g.id, auto: box.checked});
        toast(box.checked ? (g.sent ? 'Auto-invite on: newcomers are sent their invitation' : 'Auto-invite on: newcomers are sent theirs once you have sent this group its invites') : 'Auto-invite off: newcomers wait in Pending for you to send');
        if (box.checked) {
          refresh();
        }
      } catch (err) {
        toast(err.message);
        box.checked = !box.checked;
      }
    });
    auto.append(box, el('span', '', 'Auto-invite'));
    const info = el('span', 'guests-group-info');
    info.title = 'Whoever comes to match this group later goes on the list; with Auto-invite on they are sent the invitation too, once you have sent this group its invites.';
    info.append(svg('info'));
    auto.append(info);
    foot.append(auto);
    const remove = el('button', 'guests-action is-remove');
    remove.type = 'button';
    remove.title = 'Take the group off';
    remove.append(svg('trash'));
    remove.addEventListener('click', async () => {
      if (!confirm('Remove this group? Anyone already sent an invitation will stay, but pending guests will be removed.')) {
        return;
      }
      try {
        const made = await post('DELETE', '/api/calendar/invites/group', {id: e.id, group: g.id});
        toast(made.dropped ? `Group removed, and ${made.dropped} with it` : 'Group removed');
        refresh();
      } catch (err) {
        toast(err.message);
      }
    });
    foot.append(remove);
    row.append(foot);
    return row;
  };
  // The sentences name tags and lists by their keys until the choices
  // are fetched; then the section is drawn again.
  if ((view.groups || []).length && !ruleOptions) {
    fetch('/api/calendar/invites/options').then(r => (r.ok ? r.json() : null)).then(options => {
      if (options && section.isConnected) {
        ruleOptions = options;
        section.replaceWith(guestListSection(e, view, refresh));
      }
    });
  }
  if (!view.list.length) {
    return section;
  }
  // The table: each group's people under it, folded until its head is
  // opened, then everyone else, households together - the viewer's
  // correction of any answer sent as it is picked.
  const table = el('div', 'guests-table');
  const households = new Map();
  for (const r of view.list) {
    if ((r.via || '').startsWith('group:') && (view.groups || []).some(g => 'group:' + g.id === r.via)) {
      continue;
    }
    const key = r.household || r.key;
    if (!households.has(key)) {
      households.set(key, []);
    }
    households.get(key).push(r);
  }
  const groups = [...households.values()].sort((a, b) => (a[0].name || '').localeCompare(b[0].name || ''));
  // guestRow is one person on the list: face, name and line, the marks,
  // the answer to correct, and the tools.
  const guestRow = r => {
    const row = el('div', 'guests-row' + (r.guestOf ? ' is-guest' : '') + (r.invited ? '' : ' is-link'));
    row.append(face(r));
    const who = el('div', 'invite-who');
    const name = el('div', 'invite-name', r.name || r.email);
    who.append(name);
    const bits = [r.guestOf ? `Guest of ${r.guestOfName}` : r.line, r.email && r.outside ? r.email : ''].filter(Boolean);
    if (bits.length) {
      who.append(el('div', 'invite-line', bits.join(' · ')));
    }
    const marks = el('div', 'guests-marks');
    if (r.ticket) {
      const chip = el('span', 'guests-chip is-ticket');
      chip.append(svg(r.ticket === 'free' ? 'gift' : r.ticket === 'waitlist' ? 'clock' : 'ticket'), el('span', '', r.ticket === 'ticket' ? 'Ticket' : r.ticket === 'free' ? 'Free ticket' : 'Waitlist'));
      marks.append(chip);
    } else if (view.party && r.invited) {
      marks.append(el('span', 'guests-chip is-noticket', 'No ticket'));
    }
    const via = viaLabel(r, view);
    if (via) {
      marks.append(el('span', 'guests-chip', via));
    }
    if (r.invited) {
      const sent = el('span', 'guests-chip ' + (r.sent ? 'is-sent' : 'is-unsent'));
      sent.append(svg(r.sent ? 'check' : 'clock'), el('span', '', r.sent ? 'Sent ' + stamp(r.sent) : 'Not sent'));
      marks.append(sent);
    }
    if (r.opened) {
      const opened = el('span', 'guests-chip is-opened');
      opened.title = 'Opened the invitation ' + r.opened;
      opened.append(svg('eye'), el('span', '', 'Opened ' + stamp(r.opened)));
      marks.append(opened);
    }
    if (r.warning) {
      marks.append(warningChip(e, r, refresh));
    }
    who.append(marks);
    row.append(who);
    const side = el('div', 'guests-side');
    side.append(answerButtons(r, e, () => refresh()));
    if (r.answer) {
      side.append(el('div', 'guests-by', answeredWords(r)));
    }
    row.append(side);
    const actions = el('div', 'guests-actions');
    if (r.link) {
      const copy = el('button', 'guests-action');
      copy.type = 'button';
      copy.title = 'Copy their own page\u2019s link - it needs no sign-in';
      copy.append(svg('link'));
      copy.addEventListener('click', () => copyText(location.origin + r.link, 'Link copied - theirs alone, no sign-in needed'));
      actions.append(copy);
    }
    if (!r.guestOf && r.invited && !r.outside) {
      const plus = el('button', 'guests-action');
      plus.type = 'button';
      plus.title = 'Add a guest for ' + firstName(r);
      plus.append(svg('plus'));
      plus.addEventListener('click', () => openGuestForm(e, r.key, refresh));
      actions.append(plus);
    }
    if (r.invited) {
      const remove = el('button', 'guests-action is-remove');
      remove.type = 'button';
      remove.title = 'Take off the list';
      remove.append(svg('trash'));
      remove.addEventListener('click', async () => {
        if (!confirm(`Take ${r.name || r.email} off the list? Their answer goes with them${r.via && r.via.startsWith('group:') ? ', and the group will not add them back' : ''}.`)) {
          return;
        }
        try {
          await post('DELETE', '/api/calendar/invites/people', {id: e.id, email: r.key});
          refresh();
        } catch (err) {
          toast(err.message);
        }
      });
      actions.append(remove);
    }
    row.append(actions);
    return row;
  };
  // Groups, each a card with its people folded under it.
  if ((view.groups || []).length) {
    const groupsHead = el('div', 'guests-part-head');
    groupsHead.append(el('div', 'guests-part-title', 'Groups'), el('div', 'guests-part-sub', 'People from these groups are included.'));
    table.append(groupsHead);
  }
  for (const g of view.groups || []) {
    const members = view.list.filter(r => r.via === 'group:' + g.id);
    const block = el('div', 'guests-household guests-group-block' + (openGroups.has(g.id) ? ' is-open' : ''));
    block.append(groupHead(g, members.length, () => {
      if (openGroups.has(g.id)) {
        openGroups.delete(g.id);
      } else {
        openGroups.add(g.id);
      }
      block.classList.toggle('is-open', openGroups.has(g.id));
    }));
    const inner = el('div', 'guests-group-members');
    for (const r of members.sort((x, y) => (x.name || '').localeCompare(y.name || ''))) {
      inner.append(guestRow(r));
    }
    block.append(inner);
    table.append(block);
  }
  // Individuals: everyone added one at a time, by their link, or as a
  // guest, households together.
  if (groups.length) {
    const singlesHead = el('div', 'guests-part-head');
    singlesHead.append(el('div', 'guests-part-title', 'Individuals'), el('div', 'guests-part-sub', 'Added one at a time, or came by the link.'));
    table.append(singlesHead);
  }
  // Each a plain row - face, name, their line, a small mark for their
  // answer - that opens their card, where the answer is changed.
  const plainRow = r => {
    const row = el('button', 'guests-plain' + (r.guestOf ? ' is-guest' : ''));
    row.type = 'button';
    row.append(face(r));
    const who = el('div', 'invite-who');
    who.append(el('div', 'invite-name', r.name || r.email));
    const bits = [r.guestOf ? `Guest of ${r.guestOfName}` : r.line, r.invited && !r.sent ? 'Not sent' : r.opened ? 'Opened' : ''].filter(Boolean);
    if (bits.length) {
      who.append(el('div', 'invite-line', bits.join(' \u00b7 ')));
    }
    row.append(who);
    if (r.warning) {
      const warn = el('span', 'guests-plain-warn');
      warn.title = r.warningWords;
      warn.append(svg('info'));
      row.append(warn);
    }
    const said = el('span', 'invite-said-mark guests-plain-mark is-' + (r.answer || 'none'));
    said.title = r.answer ? answerWords[r.answer] : 'No response';
    said.append(svg(r.answer === 'yes' ? 'check' : r.answer === 'maybe' ? 'clock' : r.answer === 'no' ? 'close' : 'info'));
    row.append(said);
    row.addEventListener('click', () => openGuestCard(e, r, view, refresh));
    return row;
  };
  for (const group of groups) {
    const block = el('div', 'guests-household guests-plain-list');
    for (const r of group) {
      block.append(plainRow(r));
    }
    table.append(block);
  }
  section.append(table);
  // The list as a table to filter, and copy from, in a popup.
  const open = el('button', 'guests-table-row');
  open.type = 'button';
  const openMark = el('div', 'guests-table-mark');
  openMark.append(svg('menu'));
  const openWords = el('div', 'guests-table-words');
  openWords.append(el('div', 'guests-table-title', 'Guest table'), el('div', 'guests-table-sub', 'View and manage your full guest list.'));
  open.append(openMark, openWords, svg('chevron'));
  open.addEventListener('click', () => openTable(e, view, refresh));
  section.append(open);
  // Once the invites are out, the way to call the event off - a party's
  // invitation may be deleted at any time; the party is Celebrate's.
  if (view.sent) {
    const foot = el('div', 'guests-cancel');
    foot.append(view.party
      ? button('Delete invite', 'trash', 'link-button danger', () => deleteInvitation(e, view))
      : button('Cancel event', 'close', 'link-button danger', () => openCancel(e, view, refresh)));
    section.append(foot);
  }
  return section;
}

// deleteInvitation takes the guest list back - and a hand-added event
// with it - after a word of warning, and goes where the event was.
async function deleteInvitation(e, view) {
  const words = view.party
    ? 'Delete the invitation? The guest list and every answer go; the party itself stays on Helios Celebrate.'
    : 'Delete this event? It comes off the calendar with its guest list, as if it had never been made.';
  if (!confirm(words)) {
    return;
  }
  try {
    const made = await post('POST', '/api/calendar/invites/delete', {id: e.id});
    toast(made.event ? 'Event deleted' : 'Invitation deleted');
    if (made.event) {
      location.href = '/';
    } else {
      const {load} = await import('./app.js');
      await load();
    }
  } catch (err) {
    toast(err.message);
  }
}

// openCancel asks how to call the event off: cancel and tell the guests
// - with a note on why, when the host has one - cancel without a word,
// or leave it be.
function openCancel(e, view, refresh) {
  const box = el('div', 'cancel-box');
  const sent = view.list.filter(r => r.invited && r.sent && r.email).length;
  box.append(el('p', 'hint', `${sent} ${sent === 1 ? 'person has' : 'people have'} been sent the invitation. Cancelling takes the event off everyone\u2019s calendar and lists; its page stays, saying it was cancelled.`));
  const choices = el('div', 'cancel-choices');
  const noteWrap = el('div', 'cancel-note');
  noteWrap.hidden = true;
  const note = el('textarea');
  note.rows = 3;
  note.placeholder = 'A word on why, if you like - it goes in the email.';
  const status = el('span', 'save-status');
  let shut = null;
  const cancel = async notify => {
    try {
      const made = await post('POST', '/api/calendar/events/cancel', {id: e.id, notify, note: note.value});
      toast(notify ? `Cancelled - ${made.told} ${made.told === 1 ? 'person' : 'people'} told` : 'Cancelled', 4000);
      shut();
      refresh();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
    }
  };
  const notify = button('Cancel event & notify guests', 'mail', 'button', () => {
    if (noteWrap.hidden) {
      noteWrap.hidden = false;
      note.focus();
      return;
    }
    cancel(true);
  });
  choices.append(notify);
  noteWrap.append(el('label', '', 'Note to the guests'), note, button('Send the cancellation', 'mail', 'button button-small', () => cancel(true)));
  choices.append(noteWrap);
  choices.append(button('Cancel event & don\u2019t notify', 'close', 'button button-secondary', () => cancel(false)));
  choices.append(button('Don\u2019t do anything', null, 'button button-secondary', () => shut()));
  box.append(choices, status);
  shut = popup('Cancel this event?', box).shut;
}

// listFilters is the bar of filters every list of people has - a search
// box, a chip per kind of person on the list (students, parents, staff,
// non-Helios, guests) to keep or drop, and dropdown checklists for RSVP,
// Grade, Classroom and, on a party, Ticket - the rule editor's own
// widgets. It calls onChange with whoever the filters leave, as they
// change; answer starts the RSVP filter on one answer, and rsvp false
// leaves that dropdown out, where the list is grouped by answer already.
function listFilters(all, view, onChange, {answer = '', opened = '', rsvp = true} = {}) {
  const {chipToggle, filterControl} = filterWidgets({el, svg, button});
  const bar = el('div', 'guest-table-bar');
  const search = el('input', 'rule-search');
  search.type = 'search';
  search.placeholder = 'Search';
  const whoOf = r => r.isStudent ? 'student' : r.isParent ? 'parent' : r.isStaff ? 'staff' : r.guestOf ? 'guest' : 'outside';
  const kinds = [['student', 'Students'], ['parent', 'Parents'], ['staff', 'Staff'], ['outside', 'Non-Helios'], ['guest', 'Guests']].filter(([key]) => all.some(r => whoOf(r) === key));
  const kept = new Set(kinds.map(([key]) => key));
  const answers = new Set(answer ? [answer] : []);
  const gradeRank = g => /^k/i.test(g) ? 0 : Number.parseInt(g.replace(/^Grade /, ''), 10) || 100;
  const grades = new Set();
  const gradeValues = [...new Set(all.flatMap(r => r.grades || []))].sort((a, b) => gradeRank(a) - gradeRank(b));
  const rooms = new Set();
  const roomValues = [...new Set(all.flatMap(r => r.classrooms || []))].sort();
  const tickets = new Set();
  const opens = new Set(opened ? [opened] : []);
  const shown = () => {
    const q = search.value.trim().toLowerCase();
    return all.filter(r => (!q || (r.name || '').toLowerCase().includes(q) || (r.email || '').toLowerCase().includes(q))
      && kept.has(whoOf(r))
      && (!answers.size || answers.has(r.answer || 'none'))
      && (!grades.size || (r.grades || []).some(g => grades.has(g)))
      && (!rooms.size || (r.classrooms || []).some(c => rooms.has(c)))
      && (!tickets.size || tickets.has(r.ticket || ''))
      && (!opens.size || opens.has(r.opened ? 'yes' : 'no')));
  };
  const changed = () => onChange(shown());
  const chips = el('div', 'chip-row');
  for (const [key, label] of kinds) {
    chips.append(chipToggle(label, true, on => {
      if (on) {
        kept.add(key);
      } else {
        kept.delete(key);
      }
      changed();
    }, key));
  }
  // The facets behind one Filter button: RSVP, Grade, Classroom and, on
  // a party, Ticket - each only where there is something to choose.
  const sections = [];
  if (rsvp) {
    sections.push({label: 'RSVP', icon: 'check', values: [{value: 'yes', label: 'Yes'}, {value: 'maybe', label: 'Maybe'}, ...(view.host ? [{value: 'no', label: 'No'}] : []), {value: 'none', label: 'No response'}], chosen: answers});
  }
  if (gradeValues.length) {
    sections.push({label: 'Grade', icon: 'school', values: gradeValues, chosen: grades});
  }
  if (roomValues.length) {
    sections.push({label: 'Classroom', icon: 'classrooms', values: roomValues, chosen: rooms});
  }
  if (view.party && view.host) {
    sections.push({label: 'Ticket', icon: 'ticket', values: [{value: 'ticket', label: 'Purchased ticket'}, {value: 'free', label: 'Free ticket'}, {value: 'waitlist', label: 'Waitlist'}, {value: '', label: 'No ticket'}], chosen: tickets});
  }
  if (view.host && view.sent && rsvp) {
    sections.push({label: 'Opened', icon: 'eye', values: [{value: 'yes', label: 'Opened the invitation'}, {value: 'no', label: 'Not yet'}], chosen: opens});
  }
  search.addEventListener('input', changed);
  bar.append(search, chips);
  if (sections.length) {
    const facets = el('div', 'guest-table-facets');
    facets.append(filterControl(sections, changed));
    bar.append(facets);
  }
  if (answer || opened) {
    onChange(shown());
  }
  return bar;
}

// openTable is the guest list as a table in a popup - name, email, grade,
// RSVP and, on a party, ticket - filtered the way Who? filters people: a
// chip per kind of person to keep or drop, dropdown checklists for RSVP,
// Grade and Classroom, and a search box. A host clicks a guest's RSVP to
// change it. Copy table and Copy emails take whoever the filters leave,
// the table as tab-separated lines for a spreadsheet and the addresses
// for a mail.
function openTable(e, view, refresh, {answer = '', opened = ''} = {}) {
  const box = el('div');
  const rows = el('div', 'guest-table-wrap');
  const count = el('div', 'picker-note');
  let shownNow = view.list;
  const shown = () => shownNow;
  const columns = ['Name', 'Email', 'Grade', 'RSVP', ...(view.party ? ['Ticket'] : []), ...(view.sent ? ['Opened'] : [])];
  // saidCell is a guest's answer: a word, or for a host a button that
  // opens Yes, Maybe and No in place.
  const saidCell = r => {
    const cell = el('td', 'guest-table-said');
    const paintCell = () => {
      cell.replaceChildren();
      cell.className = 'guest-table-said is-' + (r.answer || 'none');
      const word = r.answer ? answerWords[r.answer] : 'No response';
      if (!(view.host && r.mine)) {
        cell.textContent = word;
        return;
      }
      const b = el('button', 'guest-table-answer', word);
      b.type = 'button';
      b.title = 'Change their answer';
      b.addEventListener('click', () => {
        cell.replaceChildren(answerButtons(r, e, () => {
          paintCell();
          // The page behind follows; the popup stays open.
          refresh();
        }));
      });
      cell.append(b);
    };
    paintCell();
    return cell;
  };
  const paint = () => {
    rows.replaceChildren();
    const list = shown();
    count.textContent = list.length === view.list.length ? `${list.length} on the list` : `${list.length} of ${view.list.length} on the list`;
    if (!list.length) {
      rows.append(el('div', 'picker-note', 'Nobody matches.'));
      return;
    }
    const table = el('table', 'guest-table');
    const thead = el('thead');
    const head = el('tr');
    for (const h of columns) {
      head.append(el('th', '', h));
    }
    thead.append(head);
    table.append(thead);
    const body = el('tbody');
    for (const r of list) {
      const tr = el('tr');
      const name = el('td', 'guest-table-name');
      name.append(el('div', '', r.name || r.email || ''));
      if (r.line) {
        name.append(el('div', 'invite-line', r.line));
      }
      tr.append(name, el('td', 'guest-table-email', r.email || ''), el('td', '', (r.grades || []).join(', ')), saidCell(r));
      if (view.party) {
        tr.append(el('td', '', r.ticket ? ticketWords(r.ticket) : ''));
      }
      if (view.sent) {
        tr.append(el('td', 'guest-table-opened', r.opened ? stamp(r.opened) : r.sent ? '\u2014' : ''));
      }
      body.append(tr);
    }
    table.append(body);
    rows.append(table);
  };
  const bar = listFilters(view.list, view, list => {
    shownNow = list;
    paint();
  }, {answer, opened});
  box.append(bar, count, rows);
  paint();
  const actions = el('div', 'modal-actions guest-table-actions');
  actions.append(button('Copy table', 'copy', 'link-button', () => {
    const lines = [['Name', 'Details', 'Email', 'Grade', 'Classroom', 'RSVP', 'Answered by', 'Answered how', 'Answered when', view.party ? 'Ticket' : 'Invited', 'Sent', 'Opened'].join('\t')];
    for (const r of shown()) {
      lines.push([r.name || '', r.guestOf ? `Guest of ${r.guestOfName}` : r.line || '', r.email || '', (r.grades || []).join(', '), (r.classrooms || []).join(', '), answerWords[r.answer] || '', r.answeredBy || '', r.answer ? (r.answeredVia === 'calendar' ? 'Calendar app' : 'Page') : '', r.answeredAt || '', view.party ? r.ticket || '' : r.invited ? 'Yes' : 'By link', stamp(r.sent), r.opened || ''].join('\t'));
    }
    copyText(lines.join('\n'), 'Table copied');
  }));
  actions.append(button('Copy emails', 'copy', 'link-button', () => {
    copyText([...new Set(shown().map(r => r.email).filter(Boolean))].join(', '), 'Addresses copied');
  }));
  box.append(actions);
  popup('Guest table', box, {wide: true});
}

// openSettings is the invitation itself, to edit in a popup: the hosts'
// message on it, whether guests may come, who reads who is coming, and
// the co-hosts. The event's own words - title, when, where - are edited
// with Edit event at the page's top, or on Celebrate for a party.
export function openSettings(e, view, refresh) {
  const s = view.settings || {audience: 'both', guests: true, guestList: 'public', message: '', hosts: []};
  const form = el('form', 'admin-form');
  const field = (label, input, note) => {
    const wrap = el('div', 'field');
    wrap.append(el('span', '', label), input);
    if (note) {
      wrap.append(el('small', '', note));
    }
    return wrap;
  };
  // A party's invitation may say the event its own way: a title, when,
  // where and a description, each blank for the party's own.
  let details = null;
  if (view.party) {
    const text = (value, placeholder) => {
      const input = el('input');
      input.type = 'text';
      input.value = value || '';
      input.placeholder = placeholder || '';
      return input;
    };
    // The form shows what the invitation says now - the party's own words
    // where it has none of its own - and saves only what differs from the
    // party's, so what is left as the party has it keeps following it.
    const o = view.original || {title: e.title, start: e.start, end: e.end, location: e.location || '', description: e.description || ''};
    const title = text(s.title || o.title);
    const startDate = el('input');
    startDate.type = 'date';
    const startTime = el('input');
    startTime.type = 'time';
    const endDate = el('input');
    endDate.type = 'date';
    const endTime = el('input');
    endTime.type = 'time';
    const start = s.start || o.start;
    const end = s.start ? s.end : o.end;
    if (start) {
      startDate.value = start.slice(0, 10);
      startTime.value = start.slice(11, 16);
      endDate.value = (end || start).slice(0, 10);
      endTime.value = (end || '').slice(11, 16);
    }
    const location = text(s.location || o.location);
    const description = el('textarea');
    description.rows = 3;
    description.value = s.description || o.description;
    const block = el('div', 'invite-details');
    block.append(field('Title', title));
    const whenRow = el('div', 'admin-when');
    whenRow.append(field('Starts', startDate), field('At', startTime), field('Ends', endDate), field('Until', endTime));
    block.append(whenRow, field('Location', location), field('Description', description));
    form.append(block);
    const when = (date, time) => (date ? date + (time ? ' ' + time : '') : '');
    const differs = (value, own) => (value === own ? '' : value);
    details = () => {
      const start = when(startDate.value, startTime.value);
      const end = startDate.value ? when(endDate.value || startDate.value, endTime.value || startTime.value) : '';
      const sameWhen = start === o.start && (end === o.end || (!o.end && end === start));
      return {
        title: differs(title.value.trim(), o.title), location: differs(location.value.trim(), o.location), description: differs(description.value.trim(), o.description),
        start: sameWhen ? '' : start, end: sameWhen ? '' : end,
      };
    };
  }
  // The email's own words, behind Add Email Invitation text until there
  // are some.
  const message = el('textarea');
  message.rows = 4;
  message.value = s.message || '';
  message.placeholder = 'A few words on the invitation - what to bring, where to park\u2026';
  const messageField = field('Email invitation text', message, 'Goes on the invitation email under the event, before its description.');
  const addMessage = button('Add Email Invitation text', 'plus', 'link-button', () => {
    addMessage.replaceWith(messageField);
    message.focus();
  });
  form.append(s.message ? messageField : addMessage);
  // A flyer: chosen here, uploaded at once, and kept with Save.
  let flyer = null;
  const flyerRow = el('div', 'settings-flyer');
  const flyerFile = el('input');
  flyerFile.type = 'file';
  flyerFile.accept = 'image/*';
  flyerFile.hidden = true;
  const flyerButton = el('label', 'link-button');
  flyerButton.append(svg('plus'), el('span', '', view.flyer ? 'Replace flyer' : 'Add flyer'), flyerFile);
  const flyerNote = el('span', 'settings-flyer-note');
  flyerFile.addEventListener('change', async () => {
    if (!flyerFile.files.length) {
      return;
    }
    flyerNote.textContent = 'Uploading\u2026';
    try {
      const {uploadImage} = await import('./images.js');
      const made = await uploadImage(flyerFile.files[0]);
      flyer = made.name;
      flyerNote.textContent = `${flyerFile.files[0].name} - saved with the invitation.`;
    } catch (err) {
      flyerNote.textContent = err.message;
    }
  });
  flyerRow.append(flyerButton, flyerNote);
  form.append(flyerRow);
  let guestList = s.guestList;
  form.append(field('Who can see who is coming', choice([{key: 'public', label: 'Helios guests'}, {key: 'private', label: 'Hosts only'}], guestList, key => {
    guestList = key;
  })));
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  actions.append(submit, status);
  form.append(actions);
  let shut = null;
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    submit.disabled = true;
    try {
      const own = details ? details() : {};
      await post('PUT', '/api/calendar/invites/settings', {id: e.id, guestList, message: message.value, ...(flyer ? {flyer} : {}), ...own});
      toast('Saved');
      shut();
      await refresh();
      // The invitation's own details changed, and the invites are out:
      // offer to send them again.
      if (details) {
        const before = {title: s.title || '', start: s.start || '', end: s.end || '', location: s.location || ''};
        const changed = ['title', 'start', 'end', 'location'].filter(key => (own[key] || '') !== before[key]);
        if (changed.length && view.sent) {
          offerUpdate(e, changed);
        }
      }
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      submit.disabled = false;
    }
  });
  shut = popup('Edit the invitation', form, {wide: Boolean(view.party)}).shut;
}

// offerUpdate asks, once the details an invitation carries have changed
// on an event whose invites are out, whether to send everyone who has it
// the invitation again as an update - its calendar invite replacing the
// one they have.
export async function offerUpdate(e, changed) {
  const view = await fetchInvites(e);
  if (!view || !view.host || !view.sent) {
    return;
  }
  const people = view.list.filter(r => r.invited && r.sent && r.email);
  if (!people.length) {
    return;
  }
  const words = {title: 'the title', start: 'the date or time', end: 'the end time', location: 'the location'};
  const what = [...new Set(changed.map(k => words[k]).filter(Boolean))].join(', ').replace(/, ([^,]*)$/, ' and $1');
  const box = el('div');
  box.append(el('p', 'hint', `You changed ${what}. ${people.length} ${people.length === 1 ? 'person has' : 'people have'} the invitation already - send it again with the new details? Their calendar invite is replaced with the new one.`));
  const actions = el('div', 'modal-actions');
  let shut = null;
  actions.append(button(`Send the update to ${people.length}`, 'mail', 'button', async () => {
    try {
      const made = await post('POST', '/api/calendar/invites/send', {id: e.id, to: 'sent', update: true});
      toast(made.messages === 1 ? 'The update is on its way' : `${made.messages} updates are on their way`, 4000);
      shut();
    } catch (err) {
      toast(err.message);
    }
  }), button('Not now', null, 'button button-secondary', () => shut()));
  box.append(actions);
  shut = popup('Send an updated invitation?', box).shut;
}

// fetchPickerData is what the Add Person tab is built from: everyone in
// the directory, and who is on the list already.
async function fetchPickerData(e) {
  const res = await fetch('/api/calendar/invites/people?id=' + encodeURIComponent(e.id));
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

// The rule editor every app shares (rules.js), given the calendar's dom
// helpers and the choices the server offers this host.
let ruleOptions = null;

const rules = rulesEditor({
  el, svg,
  options: () => ruleOptions || {classrooms: [], grades: [], tags: [], lists: [], shared: [], roles: ['Student', 'Parent', 'Staff'], relations: ['Parents', 'Children', 'Siblings']},
  me: () => ({email: me().email}),
  personName: () => '',
});

// groupWords is a group's rule as a sentence.
export function groupWords(g) {
  const options = ruleOptions || {lists: [], shared: [], tags: []};
  const labels = t => {
    const list = options.lists.find(l => l.key === t);
    const shared = options.shared.find(x => x.key === t);
    return list ? list.name : shared ? `${shared.name} (${shared.ownerName}'s)` : t;
  };
  return rules.ruleWords({...g.rule, tagLabels: g.rule.tags.map(labels)});
}

// openPicker is the way onto the list, in a popup: one tab per way of
// adding - Add Person, one at a time from the directory; Add Group, a
// rule whose matches come on now and as they come, Auto-invite saying
// whether a newcomer is sent theirs or waits in Pending; Add Non-Helios, a name and an address from outside.
async function openPicker(e, view, refresh) {
  let data = null;
  try {
    [data, ruleOptions] = await Promise.all([fetchPickerData(e), fetch('/api/calendar/invites/options').then(r => (r.ok ? r.json() : null))]);
  } catch (err) {
    toast(err.message);
    return;
  }
  const onList = new Set(data.onList || []);
  const box = el('div', 'picker');
  const tabs = el('div', 'tabs');
  const panel = el('div', 'picker-panel');
  const kinds = [['person', 'Add Person'], ['group', 'Add Group'], ['outside', 'Add Non-Helios']];
  let active = 'person';
  for (const [key, label] of kinds) {
    const b = el('button', 'tab-button' + (key === active ? ' is-active' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      active = key;
      for (const t of tabs.children) {
        t.classList.toggle('is-active', t === b);
      }
      paintPanel();
    });
    tabs.append(b);
  }
  let shut = null;
  const done = async words => {
    toast(words, 5000);
    shut();
    refresh();
  };
  const paintPanel = () => {
    panel.replaceChildren();
    switch (active) {
      case 'person':
        panel.append(personPanel(e, data, onList, done));
        break;
      case 'group':
        panel.append(groupPanel(e, done));
        break;
      case 'outside':
        panel.append(outsidePanel(e, onList, done));
        break;
    }
  };
  box.append(tabs, panel);
  paintPanel();
  shut = popup('Add to the guest list', box, {wide: true}).shut;
}

// personPanel is the directory as Who? lists it: a search box, chips to
// keep to students, parents or staff, Add family at the right - their
// parents, children or siblings, who come along with each pick and are
// named on their line - and a row per person - face, name, their line -
// that a click picks; the picked go on together.
function personPanel(e, data, onList, done) {
  const wrap = el('div');
  const byEmail = new Map(data.people.map(p => [p.email, p]));
  const picked = new Map();
  const family = new Set();
  const search = el('input', 'picker-search');
  search.type = 'search';
  search.placeholder = 'Search by name or email';
  wrap.append(search);
  const roles = new Set();
  const bar = el('div', 'picker-bar');
  const chips = el('div', 'filter-chips picker-roles');
  for (const [role, label, test] of [['student', 'Students', p => p.isStudent], ['parent', 'Parents', p => p.isParent], ['staff', 'Staff', p => p.isStaff]]) {
    const chip = el('button', 'filter-chip', label);
    chip.type = 'button';
    chip.test = test;
    chip.addEventListener('click', () => {
      if (roles.has(role)) {
        roles.delete(role);
      } else {
        roles.add(role);
      }
      chip.classList.toggle('is-on', roles.has(role));
      paintList();
    });
    chips.append(chip);
  }
  bar.append(chips);
  // The relations, as the rule editor offers them.
  const relations = [{value: 'Parents', label: 'Their parents'}, {value: 'Children', label: 'Their children'}, {value: 'Siblings', label: 'Their siblings'}];
  bar.append(rules.facetDropdown('Add family', 'families', relations, family, () => {
    paintList();
    paintButton();
  }));
  wrap.append(bar);
  // relativesOf is who comes along with a person under the relations
  // chosen, not on the list already.
  const relativesOf = p => {
    const out = [];
    for (const relation of ['Parents', 'Children', 'Siblings']) {
      if (!family.has(relation)) {
        continue;
      }
      for (const email of p[relation.toLowerCase()] || []) {
        const r = byEmail.get(email);
        if (r && !onList.has(email) && !out.includes(r)) {
          out.push(r);
        }
      }
    }
    return out;
  };
  // everyone is the picked and their relatives, each once.
  const everyone = () => {
    const out = new Map();
    for (const p of picked.values()) {
      out.set(p.email, {email: p.email, name: p.name, via: 'search'});
      for (const r of relativesOf(p)) {
        if (!out.has(r.email)) {
          out.set(r.email, {email: r.email, name: r.name, via: 'family'});
        }
      }
    }
    return out;
  };
  const list = el('div', 'picker-results');
  const foot = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const add = el('button', 'button');
  add.type = 'button';
  const paintButton = () => {
    const n = everyone().size;
    add.replaceChildren(svg('plus'), el('span', '', n ? `Add ${n} ${n === 1 ? 'person' : 'people'}` : 'Add to the list'));
    add.disabled = !n;
  };
  const paintList = () => {
    list.replaceChildren();
    const q = search.value.trim().toLowerCase();
    const tests = [...chips.children].filter(c => c.classList.contains('is-on')).map(c => c.test);
    const found = data.people.filter(p => (!q || (p.name || '').toLowerCase().includes(q) || p.email.toLowerCase().includes(q)) && (!tests.length || tests.some(t => t(p))));
    if (!found.length) {
      list.append(el('div', 'picker-note', 'Nobody by that name. Someone outside Helios goes on Add Non-Helios.'));
    }
    for (const p of found.slice(0, 200)) {
      const on = onList.has(p.email);
      const row = el('button', 'picker-person' + (on ? ' is-on' : picked.has(p.email) ? ' is-picked' : ''));
      row.type = 'button';
      row.disabled = on;
      const mark = el('span', 'picker-check');
      mark.append(svg('check'));
      row.append(mark, face(p));
      const who = el('div', 'invite-who');
      who.append(el('div', 'invite-name', p.name || p.email));
      const along = relativesOf(p).map(r => firstName(r));
      const line = [on ? 'On the list' : p.line, along.length ? 'with ' + along.join(', ') : ''].filter(Boolean).join(' \u00b7 ');
      if (line) {
        const words = el('div', 'invite-line', line);
        if (along.length) {
          words.classList.add('has-family');
        }
        who.append(words);
      }
      row.append(who);
      row.addEventListener('click', () => {
        if (picked.has(p.email)) {
          picked.delete(p.email);
        } else {
          picked.set(p.email, p);
        }
        row.classList.toggle('is-picked', picked.has(p.email));
        paintButton();
      });
      list.append(row);
    }
    if (found.length > 200) {
      list.append(el('div', 'picker-note', `${found.length - 200} more - type a name to narrow it.`));
    }
  };
  search.addEventListener('input', paintList);
  add.addEventListener('click', async () => {
    add.disabled = true;
    try {
      const made = await post('POST', '/api/calendar/invites/people', {id: e.id, people: [...everyone().values()]});
      done(`${made.added} added to the list.`);
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      add.disabled = false;
    }
  });
  foot.append(add, status);
  wrap.append(list, foot);
  paintList();
  paintButton();
  setTimeout(() => search.focus(), 0);
  return wrap;
}

// groupPanel is one rule in the editor every app shares - roles, words,
// classrooms, grades, tags and lists, family - read back as a sentence
// with who it picks out now, and Auto-invite new members, on to start,
// so whoever comes to match it later is put on the list and sent the
// invitation too.
function groupPanel(e, done) {
  const wrap = el('div', 'picker-group');
  if (!ruleOptions) {
    wrap.append(el('div', 'picker-note', 'Groups are not set up on this server.'));
    return wrap;
  }
  const rule = rules.newRule('include');
  const holder = el('div', 'rule is-include picker-rule');
  const preview = el('div', 'audience-preview picker-preview');
  let timer = null;
  const askPreview = () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (!rules.ruleSaysSomething(rule)) {
        preview.textContent = '';
        return;
      }
      try {
        const answer = await post('POST', '/api/calendar/invites/preview', {rule});
        preview.textContent = answer.count ? `Picks out ${answer.count} ${answer.count === 1 ? 'person' : 'people'} now: ${answer.names.join(', ')}${answer.count > answer.names.length ? '…' : ''}` : 'Picks out nobody yet.';
      } catch (err) {
        preview.textContent = err.message;
      }
    }, 300);
  };
  holder.append(rules.ruleControls(rule, askPreview));
  wrap.append(holder, preview);
  const auto = el('label', 'picker-auto');
  const box = el('input');
  box.type = 'checkbox';
  box.checked = true;
  auto.append(box, el('span', '', 'Auto-invite new members'), el('small', '', 'Whoever comes to match this later - a family joining the classroom, a ticket sold - goes on the list either way. With this on they are sent their invitation too, once you have sent this group its invitations yourself; off, they wait in Pending for you.'));
  wrap.append(auto);
  const foot = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const add = button('Add group', 'plus', 'button', async () => {
    if (!rules.ruleSaysSomething(rule)) {
      status.textContent = 'Pick a role, some words, a classroom, a grade or a tag.';
      status.classList.add('error');
      return;
    }
    add.disabled = true;
    try {
      const made = await post('POST', '/api/calendar/invites/group', {id: e.id, rule, auto: box.checked});
      done(`Group added, with ${made.added} ${made.added === 1 ? 'person' : 'people'} on the list now.`);
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      add.disabled = false;
    }
  });
  foot.append(add, status);
  wrap.append(foot);
  return wrap;
}

// outsidePanel takes people from outside Helios by family: each family
// a card - the one with the address at its head, then the family members
// under them, each by name and, when they have one, an address - and Add
// another family for the next. Everyone gets the email and the calendar
// invite; each with an address gets a page of their own to answer from,
// no sign-in needed, that shows the event and their family and nothing of
// who else is coming, and answers for the family there.
function outsidePanel(e, onList, done) {
  const wrap = el('div');
  wrap.append(el('div', 'picker-note', 'Someone outside Helios - a coach, a grandparent, a friend - and their family. They get the email and the calendar invite with a page of their own to answer from, no sign-in needed, that shows the event and nothing of who else is coming.'));
  const families = el('div', 'outside-families');
  const status = el('span', 'save-status');
  const emailForm = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;
  // familyCard is one family: the head's name and address, the members
  // added under them, and a way to take the card away.
  const familyCard = () => {
    const card = el('div', 'outside-family');
    const members = [];
    const head = el('div', 'picker-email');
    const name = el('input');
    name.type = 'text';
    name.placeholder = 'Name';
    const email = el('input');
    email.type = 'email';
    email.placeholder = 'Email address';
    head.append(name, email);
    card.append(head);
    const family = el('div', 'outside-members');
    family.append(el('div', 'outside-members-title', 'Family members (optional)'), el('div', 'picker-note', 'A spouse, a partner, children - whoever is coming with them. Anyone with an address gets an invitation of their own.'));
    const list = el('div', 'outside-member-list');
    const paintMembers = () => {
      list.replaceChildren();
      for (const m of members) {
        const row = el('div', 'outside-member');
        const face = el('span', 'avatar outside-member-face', m.name.split(/\s+/).map(w => w[0] || '').join('').slice(0, 2).toUpperCase());
        row.append(face, el('span', 'outside-member-name', m.name), el('span', 'outside-member-email', m.email || 'No address - answered for by the family'));
        const remove = el('button', 'guests-action is-remove');
        remove.type = 'button';
        remove.title = 'Remove';
        remove.append(svg('trash'));
        remove.addEventListener('click', () => {
          members.splice(members.indexOf(m), 1);
          paintMembers();
        });
        row.append(remove);
        list.append(row);
      }
    };
    family.append(list);
    // Add family member opens a name and an address to fill; Enter or
    // Add puts them on the card.
    const adder = el('div', 'picker-email outside-adder');
    adder.hidden = true;
    const mName = el('input');
    mName.type = 'text';
    mName.placeholder = 'Family member\u2019s name';
    const mEmail = el('input');
    mEmail.type = 'email';
    mEmail.placeholder = 'Their email (optional)';
    const put = button('Add', 'plus', 'button button-small', () => {
      const n = mName.value.trim();
      const a = mEmail.value.trim().toLowerCase();
      if (!n) {
        mName.focus();
        return;
      }
      if (a && !emailForm.test(a)) {
        mEmail.focus();
        return;
      }
      members.push({name: n, email: a});
      mName.value = '';
      mEmail.value = '';
      paintMembers();
      mName.focus();
    });
    adder.append(mName, mEmail, put);
    for (const input of [mName, mEmail]) {
      input.addEventListener('keydown', ev => {
        if (ev.key === 'Enter') {
          ev.preventDefault();
          put.click();
        }
      });
    }
    family.append(button('Add family member', 'plus', 'button button-secondary button-small', () => {
      adder.hidden = false;
      mName.focus();
    }), adder);
    card.append(family);
    card.members = members;
    card.head = () => ({name: name.value.trim(), email: email.value.trim().toLowerCase()});
    card.focus = () => name.focus();
    return card;
  };
  families.append(familyCard());
  wrap.append(families);
  wrap.append(button('Add another family', 'plus', 'link-button', () => {
    const card = familyCard();
    families.append(card);
    card.focus();
  }));
  const foot = el('div', 'modal-actions');
  const add = button('Add to the list', 'plus', 'button', async () => {
    const people = [];
    for (const card of families.children) {
      const head = card.head();
      if (!head.name && !head.email && !card.members.length) {
        continue;
      }
      if (!head.name || !emailForm.test(head.email)) {
        status.textContent = 'Each family needs a name and an email address at its head.';
        status.classList.add('error');
        card.focus();
        return;
      }
      if (onList.has(head.email)) {
        status.textContent = `${head.name} is on the list already.`;
        status.classList.add('error');
        return;
      }
      people.push({email: head.email, name: head.name, via: 'outside', household: head.email});
      for (const m of card.members) {
        people.push({email: m.email, name: m.name, via: 'outside', household: head.email});
      }
    }
    if (!people.length) {
      status.textContent = 'A name and an email address, please.';
      status.classList.add('error');
      return;
    }
    add.disabled = true;
    try {
      const made = await post('POST', '/api/calendar/invites/people', {id: e.id, people});
      done(made.added === 1 ? `${people[0].name} added to the list.` : `${made.added} people added to the list.`);
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      add.disabled = false;
    }
  });
  foot.append(add, status);
  wrap.append(foot);
  setTimeout(() => families.firstChild.focus(), 0);
  return wrap;
}

// inviteHostCall is the way into a guest list on an event that has none
// yet, for someone who may build one: a party's host on the party's page
// here, or the poster of any hand-added event.
export function inviteHostCall(e, view, refresh) {
  const card = el('div', 'guests-start');
  card.append(svg('people'));
  const words = el('div', 'guests-start-words');
  words.append(el('div', 'guests-start-title', isParty(e) ? 'Invite your guests' : 'Invite people'), el('div', 'guests-start-lead', isParty(e) ? 'Ask the ticket holders to confirm they are coming - or invite anyone else - and see who has answered beside who has a ticket.' : 'Build a guest list from the directory, a classroom, your lists or anyone by email, then send everyone the invitation with a calendar invite attached.'));
  card.append(words);
  if (isParty(e)) {
    card.append(button('Invite the ticket holders', 'ticket', 'button', async () => {
      try {
        const made = await startParty(e);
        toast(made.added ? `${made.added} ticket holders on the list - it follows the tickets from here.` : 'The list follows the tickets from here.', 5000);
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  card.append(button('Add People', 'plus', 'button' + (isParty(e) ? ' button-secondary' : ''), () => openPicker(e, view, refresh)));
  return card;
}
