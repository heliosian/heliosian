import {state, me, answer, eventDates, weekdayLong, parseDate, timeLine} from './state.js';
import {el, svg, button, iconButton, toast, longToast} from '/elements.js';
import {popup} from '/modal.js';
import {checkbox} from '/form.js';
import {createPersonPicker} from '/picker.js';
import {known} from '/directory.js';
import {uploadImage} from './imagecontrol.js';
import {answerWords, answerIcon, answerFor, firstName, ticketWords, stamp, pickerPeople} from './inviteparts.js';
import {openGuestForm, openGuestCard, openPending, sendInvites, deleteInvitation, openCancel} from './guestpopups.js';
import {listFilters, openTable, openMessage} from './guesttable.js';
import {openPicker, guestAdders} from './addpeople.js';
import {andList} from '/andlist.js';
import {personTile, personCard, peopleRow} from '/people.js';
import {personRow} from '/personrow.js';
import {familiesOf, familySwitch, familySaid, familyRow} from '/families.js';
import {countList} from '/countlist.js';
import {openPersonCard} from '/personcard.js';
import {memberAdders, membersCard} from '/members.js';
import {rulesCard, ruleWords} from '/listrules.js';
import {tabbedFields} from '/tabs.js';
import * as guests from './guestlist.js';

export async function fetchInvites(e) {
  const view = await guests.fetchInvites(e);
  if (view.can.opened) {
    guests.noteOpened(e, view).catch(err => toast(err.message));
  }
  return view;
}

function ownKey(view) {
  const self = view.mine.find(r => r.key === me().id && !r.guestOf);
  return self ? self.key : (view.mine.find(r => r.mine && !r.guestOf) || {}).key;
}

function removeGuestButton(e, view, r, className, refresh) {
  const remove = el('button', className);
  remove.type = 'button';
  remove.title = 'Remove this guest';
  remove.append(svg('close'));
  remove.addEventListener('click', async () => {
    try {
      await guests.takeOff(e, view, r);
      toast(`${r.name} removed`);
      refresh();
    } catch (err) {
      toast(err.message);
    }
  });
  return remove;
}

function hideButton(e, refresh) {
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
  return hide;
}

function bigChoices(e, r, refresh) {
  const wrap = el('div', 'rsvp-choices');
  for (const [word, label] of Object.entries(answerWords)) {
    const b = el('button', 'rsvp-choice is-' + word + (r.answer === word ? ' is-on' : ''));
    b.type = 'button';
    b.disabled = !r.mine;
    const mark = el('span', 'invite-said-mark');
    mark.append(svg(answerIcon(word)));
    b.append(mark, el('strong', '', label));
    b.addEventListener('click', async () => {
      try {
        await answerFor(e, r, r.answer === word ? '' : word);
        refresh();
      } catch (err) {
        toast(err.message);
      }
    });
    wrap.append(b);
  }
  return wrap;
}

function answerItems(e, r, refresh, clear) {
  const run = next => async () => {
    try {
      await answerFor(e, r, next);
      refresh();
    } catch (err) {
      toast(err.message);
    }
  };
  const items = Object.entries(answerWords).map(([word, words]) => ({icon: answerIcon(word), words, word, run: run(word)}));
  if (clear) {
    items.push({icon: answerIcon(''), words: clear, word: 'none', run: run('')});
  }
  return items;
}

function memberRow(e, view, r, refresh) {
  const row = el('div', 'rsvp-panel-person');
  row.append(personRow(r, {
    className: 'rsvp-person',
    lines: [r.guestOf ? `Guest of ${r.guestOfName}` : r.invitedBy ? `Invited by ${r.invitedBy}${r.invitedAt ? ' on ' + stamp(r.invitedAt) : ''}` : ''],
    gradeColors: state.model.gradeColors,
  }), bigChoices(e, r, refresh));
  if (r.guestOf && r.mine) {
    row.append(removeGuestButton(e, view, r, 'invite-remove rsvp-person-remove', refresh));
  }
  return row;
}

export function familyBand(e, view, refresh) {
  const box = el('div', 'rsvp-panel');
  const self = view.mine.find(r => r.key === me().id && !r.guestOf);
  const others = view.mine.filter(r => r !== self);
  const asked = view.mine.some(r => r.invited);
  const hosts = view.hosts.filter(h => h.key !== me().id).map(h => h.name).filter(Boolean);
  const by = asked && hosts.length && !view.hostsHidden ? `Invited by ${hosts.join(' and ')}. ` : '';
  const top = el('div', 'rsvp-panel-top');
  const icon = el('div', 'guests-table-mark');
  icon.append(svg('person'));
  const words = el('div', 'guests-start-words');
  words.append(el('div', 'guests-title', self ? 'Your RSVP' : 'Your family’s RSVP'), el('div', 'guests-start-lead', by + (self ? 'Will you be attending?' : 'Choose who from your family will be attending.')));
  top.append(icon, words);
  if (self) {
    top.append(bigChoices(e, self, refresh));
  }
  box.append(top);
  for (const r of others) {
    box.append(memberRow(e, view, r, refresh));
  }
  const foot = el('div', 'rsvp-panel-foot');
  const of = ownKey(view);
  if (of && (view.guests || view.host)) {
    foot.append(button('Add guests', 'plus', 'rsvp-clear rsvp-add', () => openGuestForm(e, of, refresh)));
  }
  foot.append(hideButton(e, refresh));
  box.append(foot);
  return box;
}

export function familyAnswered(view) {
  const mine = view.mine.filter(r => r.mine);
  return mine.length > 0 && mine.every(r => r.answer);
}

export function rsvpRow(e, view, refresh) {
  const row = el('div', 'side-row rsvp-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('calcheck'));
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', 'My RSVP'));
  for (const r of view.mine) {
    const line = el('div', 'rsvp-row-line' + (r.guestOf ? ' is-guest' : ''));
    const said = el(r.mine ? 'button' : 'span', 'invite-said rsvp-row-said is-' + (r.answer || 'none'));
    const mark = el('span', 'invite-said-mark');
    mark.append(svg(answerIcon(r.answer)));
    said.append(mark, el('strong', '', r.key === me().id ? 'You' : firstName(r)), el('span', 'rsvp-row-word', r.answer ? answerWords[r.answer] : 'No response'));
    if (r.mine) {
      said.type = 'button';
      said.title = 'Change the answer';
      said.append(svg('chevron-down'));
      line.append(dropMenu(said, answerItems(e, r, refresh, 'Clear'), true));
    } else {
      line.append(said);
    }
    if (r.guestOf && r.mine) {
      line.append(removeGuestButton(e, view, r, 'invite-remove rsvp-row-remove', refresh));
    }
    body.append(line);
  }
  const foot = el('div', 'invite-foot');
  const of = ownKey(view);
  if (of && (view.guests || view.host)) {
    foot.append(button('Add a guest', 'plus', 'link-button', () => openGuestForm(e, of, refresh)));
  }
  foot.append(hideButton(e, refresh));
  body.append(foot);
  row.append(icon, body);
  return row;
}

export function comingCard(e, view, refresh) {
  const card = el('section', 'rsvps-card coming-section');
  if (view.host) {
    const unsent = view.list.filter(r => r.pending && r.email);
    if (unsent.length) {
      const pending = el('div', 'rsvps-pending');
      const mark = el('span', 'rsvps-pending-mark');
      mark.append(svg('mail'));
      const words = el('div', 'rsvps-pending-words');
      const sub = el('div', 'rsvps-pending-sub');
      sub.append(el('span', '', `${unsent.length} ${unsent.length === 1 ? 'invitation' : 'invitations'} not sent yet`), button('More info', 'info', 'link-button rsvps-pending-info', () => openPending(e, view, refresh)));
      words.append(el('div', 'rsvps-pending-title', 'Pending'), sub);
      const send = button('Send Now', 'mail', 'button rsvps-pending-send', () => sendInvites(e, unsent.length, `Send the invitation to ${unsent.length} ${unsent.length === 1 ? 'person' : 'people'} who have not had it yet? Each gets an email with the calendar invite; a student's goes to them and their parents.`, refresh));
      pending.append(mark, words, send);
      card.append(pending);
    }
  }
  const rows = (view.host ? view.list : view.coming).filter(r => r.answer === 'yes' || (view.host && r.answer === 'no') || (r.invited && !r.answer));
  const head = el('div', 'rsvps-card-head');
  head.append(el('h2', 'section section-swoosh', 'Who’s coming'));
  card.append(head);
  if (view.host) {
    card.append(el('div', 'side-line coming-privacy', view.listPrivate ? 'Only the hosts see this list.' : 'Everyone who opens the event sees this list.'));
  }
  const grid = el('div', 'coming-grid');
  let byFamily = false;
  let current = rows;
  const unsent = r => view.host && r.pending && r.email && !r.answer;
  const groups = [
    {key: 'yes', label: 'Yes', cls: 'is-yes', has: r => r.answer === 'yes'},
    ...(view.host ? [{key: 'no', label: 'No', cls: 'is-no', has: r => r.answer === 'no'}] : []),
    {key: 'waiting', label: 'No response', cls: 'is-waiting', has: r => r.invited && !r.answer && !unsent(r)},
    ...(view.host ? [{key: 'pending', label: 'Pending (Unsent)', cls: 'is-pending', has: unsent}] : []),
  ];
  const familyRank = ['yes', 'waiting', 'pending', 'no'];
  const familyGroup = family => familyRank.map(key => groups.find(g => g.key === key)).find(g => g && family.some(g.has));
  const cardOf = p => personCard(p, {
    onClick: () => openGuestCard(e, p, view, refresh),
    line: p.guestOf ? `Guest of ${p.guestOfName}` : p.line,
    mine: view.mine.some(m => m.key === p.key),
    corner: ticketMark(view, p),
    gradeColors: state.model.gradeColors,
  });
  const paint = shown => {
    current = shown;
    grid.replaceChildren();
    const families = byFamily ? familiesOf(shown) : [];
    const filled = groups.map(g => ({...g, items: byFamily ? families.filter(f => familyGroup(f) === g) : shown.filter(g.has)}));
    if (!filled.some(g => g.items.length)) {
      grid.append(el('div', 'side-line', rows.length ? 'Nobody matches.' : 'Nobody has answered yet.'));
      return;
    }
    for (const g of filled) {
      if (!g.items.length) {
        continue;
      }
      grid.append(el('div', 'rsvps-head ' + g.cls, g.label));
      if (!byFamily) {
        const list = el('div', 'person-cards');
        list.append(...g.items.map(cardOf));
        grid.append(list);
        continue;
      }
      const list = el('div', 'family-rows');
      list.append(...g.items.map(family => guestFamilyRow(e, view, family, refresh)));
      grid.append(list);
    }
  };
  if (rows.length > 1) {
    const by = familySwitch(byFamily, family => {
      byFamily = family;
      paint(current);
    });
    const bar = listFilters(rows, view, paint, {rsvp: false});
    head.append(bar.querySelector('.rule-search'));
    bar.prepend(by);
    card.append(bar);
  }
  paint(rows);
  card.append(grid);
  return card;
}

function ticketMark(view, p) {
  if (!(view.party && view.host && p.invited)) {
    return null;
  }
  const mark = el('span', 'ticket-mark is-' + (p.ticket || 'none'));
  mark.title = ticketWords(p.ticket);
  mark.append(svg(p.ticket === 'ticket' ? 'ticket' : p.ticket === 'free' ? 'gift' : 'close'));
  return mark;
}

function guestFamilyRow(e, view, family, refresh) {
  return familyRow(family, family.map(r => {
    const unsent = view.host && r.pending && r.email && !r.answer;
    const said = familySaid(unsent ? 'pending' : r.answer, r.mine);
    const send = {icon: 'mail', words: 'Send Now', cls: 'is-send', run: async () => {
      try {
        await guests.sendTo(e, [r.key]);
        toast(`Invite sent to ${r.name || r.email}`);
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }};
    return {
      person: r,
      kind: r.guestOf ? 'guest' : r.isStudent ? 'kid' : r.isParent ? 'parent' : '',
      name: r.key === me().id ? 'You' : firstName(r),
      role: r.guestOf ? 'Guest' : r.isStudent ? r.grade || 'Student' : r.isParent ? 'Parent' : r.isStaff ? 'Staff' : '',
      onClick: () => openGuestCard(e, r, view, refresh),
      corner: ticketMark(view, r),
      after: r.mine ? dropMenu(said, [...(unsent ? [send] : []), ...answerItems(e, r, refresh, 'Clear')], true) : said,
    };
  }), state.model.gradeColors);
}

export function inviteCall(e, view, refresh) {
  if (!view.mayInvite) {
    return null;
  }
  const card = el('div', 'guests-start');
  card.append(svg('people'));
  const words = el('div', 'guests-start-words');
  words.append(el('div', 'guests-start-title', 'Invite others'), el('div', 'guests-start-lead', 'Share this event with friends and family who might be interested. They get the invitation by email.'));
  card.append(words, button('Invite others', 'people', 'button', () => openPicker(e, view, refresh)));
  return card;
}

export function hostsRow(e, view, refresh) {
  const self = me().id;
  const tiles = [];
  for (const h of view.hosts) {
    const tile = personTile(h, {
      onClick: () => openPersonCard(h),
      title: [h.name, h.line].filter(Boolean).join(' · '),
      gradeColors: state.model.gradeColors,
    });
    const own = h.key === self;
    if (view.host && (own || view.hosts.length > 1)) {
      const x = el('button', 'hosts-card-remove');
      x.type = 'button';
      x.title = own ? 'Step down as host' : `Take ${h.name} off as a host`;
      x.textContent = '×';
      x.addEventListener('click', async ev => {
        ev.stopPropagation();
        const alone = view.hosts.length === 1;
        const ask = own
          ? `Step down as host of ${e.title}? You won't be able to edit it or run its guest list any more${alone ? ', and it will have no host - an admin can still change it' : ''}.`
          : `Take ${h.name} off as a host of ${e.title}?`;
        if (!confirm(ask)) {
          return;
        }
        try {
          await guests.removeHost(e, view, h);
          toast(own ? 'You no longer host this event.' : `${h.name} no longer hosts this event.`);
          refresh();
        } catch (err) {
          toast(err.message);
        }
      });
      tile.append(x);
    }
    tiles.push(tile);
  }
  const after = [];
  if (view.host && view.managedBy) {
    const tools = el('div', 'hosts-tools');
    tools.append(button('Add co-host', 'plus', 'link-button', () => openAddHost(e, view, refresh)));
    const hidden = view.hostsHidden;
    tools.append(button(hidden ? 'Show hosts' : 'Hide hosts', hidden ? 'eye' : 'eye-off', 'link-button', async () => {
      try {
        await guests.setHostsHidden(e, view, !hidden);
        toast(hidden ? 'Everyone who opens the event sees its hosts again.' : 'Only the hosts see who hosts this event now.');
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
    after.push(tools);
  }
  return peopleRow({
    title: 'Hosts',
    line: andList(view.hosts.map(h => h.name || h.email).filter(Boolean)),
    notes: view.hostsHidden ? [el('div', 'side-line hosts-hidden-note', 'Hidden - only the hosts see this.')] : [],
    tiles,
    after,
  });
}

function openAddHost(e, view, refresh) {
  const form = el('form', 'admin-form');
  form.append(el('p', 'hint', 'A co-host builds and sends the list, reads every answer and hears replies, as you do.'));
  const mount = el('div', 'cohost-picker cohost-add');
  const picker = createPersonPicker(mount, {people: pickerPeople(p => !view.hosts.some(h => h.key === p.id))});
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
    const p = picker.value && known(picker.value);
    if (!p) {
      status.textContent = 'Pick someone from the directory first.';
      status.classList.add('error');
      return;
    }
    submit.disabled = true;
    try {
      await guests.addHost(e, view, p.id);
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
  picker.input.focus();
}

function menuButton(label, icon, items) {
  const toggle = el('button', 'button button-secondary button-small');
  toggle.type = 'button';
  toggle.append(svg(icon), el('span', '', label), svg('chevron-down'));
  return dropMenu(toggle, items);
}

function dropMenu(toggle, items, below = false) {
  const holder = el('div', below ? 'rsvp-member-holder' : 'hero-image-menu-holder');
  toggle.setAttribute('aria-haspopup', 'menu');
  const menu = el('div', 'hero-image-menu' + (below ? ' rsvp-member-menu' : ''));
  menu.hidden = true;
  for (const item of items) {
    const b = el('button', 'hero-image-menu-item' + (item.cls ? ' ' + item.cls : ''));
    b.type = 'button';
    if (item.word) {
      const said = el('span', 'invite-said is-' + item.word);
      const mark = el('span', 'invite-said-mark');
      mark.append(svg(item.icon));
      said.append(mark, el('span', '', item.words));
      b.append(said);
    } else {
      b.append(svg(item.icon), el('span', '', item.words));
    }
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

function flyerFile(e, refresh) {
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  file.addEventListener('change', async () => {
    if (!file.files.length) {
      return;
    }
    try {
      const made = await uploadImage(file.files[0]);
      await guests.setFlyer(e, made.name);
      toast('Flyer saved');
      refresh();
    } catch (err) {
      toast(err.message);
    }
  });
  return file;
}

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
    const bar = el('div', 'flyer-actions');
    const upload = el('label', 'button button-secondary button-small');
    upload.append(svg('plus'), el('span', '', 'Replace'), flyerFile(e, refresh));
    bar.append(upload, button('Remove', 'trash', 'button button-secondary button-small', async () => {
      try {
        await guests.setFlyer(e, '');
        toast('Flyer removed');
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
    card.append(bar);
  }
  return card;
}

export function addFlyerLink(e, view, refresh) {
  if (!view.host || view.flyer) {
    return null;
  }
  const link = el('label', 'side-add-flyer');
  link.append(svg('plus'), el('span', '', 'Add a flyer'), flyerFile(e, refresh));
  return link;
}

export function guestListSection(e, view, refresh) {
  const section = el('section', 'side-card guests-section');
  section.append(guestsHead(e, view, refresh), guestCounts(e, view, refresh));
  if (!view.list.length) {
    section.append(el('p', 'guests-note', 'Add people from the directory, a classroom, one of your lists, or by email - then send the invitation.'));
    return section;
  }
  if (view.sent && e.source === 'sheet') {
    const foot = el('div', 'guests-cancel');
    foot.append(button('Cancel event', 'close', 'link-button danger', () => openCancel(e, view, refresh)));
    section.append(foot);
  }
  return section;
}

export function openGuestSettings(e, view, refresh) {
  const panel = el('div');
  const again = async () => {
    refresh();
    try {
      view = await fetchInvites(e);
    } catch (err) {
      toast(err.message);
      return;
    }
    paintGuests(e, view, panel, again);
  };
  paintGuests(e, view, panel, again);
  const box = el('div', 'guest-settings');
  box.append(tabbedFields([{label: 'Guests', fields: [panel]}, {label: 'Settings', fields: [settingsPanel(e, view, refresh)]}]));
  popup('Guest List Settings', box, {wide: true});
}

function paintGuests(e, view, panel, again) {
  const draft = view.rules.map(r => ({...r}));
  const counts = new Map();
  const status = el('span', 'save-status');
  const save = button('Save rules', 'check', 'button button-small', async () => {
    save.disabled = true;
    try {
      await guests.saveRules(e, view, draft.filter(r => r.target || r.person || r.search));
      toast('Rules saved');
      again();
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      save.disabled = false;
    }
  });
  save.disabled = true;
  let timer = null;
  const changed = () => {
    save.disabled = false;
    clearTimeout(timer);
    timer = setTimeout(async () => {
      try {
        const preview = await guests.previewRules(e, view, draft.filter(r => r.target || r.person || r.search));
        counts.clear();
        draft.filter(r => r.target || r.person || r.search).forEach((r, i) => counts.set(r, preview.ruleCounts[i]));
        made.render();
      } catch (err) {
        status.textContent = err.message;
        status.classList.add('error');
      }
    }, 300);
  };
  const made = rulesCard({
    list: () => draft,
    onAdd: rule => {
      draft.push(rule);
      changed();
    },
    onChange: changed,
    onRemove: rule => {
      draft.splice(draft.indexOf(rule), 1);
      made.render();
      changed();
    },
    count: rule => counts.get(rule),
    empty: 'No rules yet: everyone on the list was added one at a time.',
    hint: 'Someone is on the guest list if a rule takes them in and no Leave out rule picks them, or if you or a guest added them by hand.',
  });
  const foot = el('div', 'modal-actions');
  foot.append(save, status);
  made.card.append(foot);
  const adders = guestAdders(e, view, words => {
    longToast(words);
    again();
  });
  const takeOff = async m => {
    const row = view.list.find(r => r.key === m.person);
    try {
      await guests.takeOff(e, view, row);
      toast(`${m.name || m.email} taken off the list`);
      again();
    } catch (err) {
      toast(err.message);
    }
  };
  const members = membersCard({
    noun: ['guest', 'guests'],
    listWord: 'guest list',
    adders: memberAdders(adders.person, adders.outside),
    phrase: ruleWords,
    chip: () => null,
    onExclude: takeOff,
    onRemove: takeOff,
    gradeColors: state.model.gradeColors,
  });
  const reasons = r => r.guestOf ? [{guestOf: r.guestOfName}] : r.byHand ? [{added: true}] : r.byRule ? [{rule: view.rules.findIndex(x => 'group:rule ' + x.order === r.via)}] : [{added: true}];
  const shown = view.list.filter(r => r.invited).map(r => ({person: r.key, email: r.email, name: r.name, photoUrl: r.photoUrl, words: r.words, grade: r.grade, outside: r.outside, reasons: reasons(r)}));
  members.show({members: shown, rules: view.rules});
  panel.replaceChildren(made.card, members.card);
}

function settingsPanel(e, view, refresh) {
  const box = el('div', 'guests-settings');
  const setting = (label, on, hint, run, words) => {
    const toggle = checkbox(label, on, hint);
    toggle.input.addEventListener('change', async () => {
      const next = toggle.input.checked;
      try {
        await run(next);
        toast(words(next));
        refresh();
      } catch (err) {
        toast(err.message);
        toggle.input.checked = !next;
      }
    });
    box.append(toggle.wrap);
  };
  setting('Show the guest list to everyone', !view.listPrivate, 'Everyone who can open the event sees who said yes, and who has not answered. Only the hosts see who said no.',
    on => guests.setPublicList(e, on), on => (on ? 'Everyone who opens the event sees who is coming now.' : 'Only the hosts see who is coming now.'));
  setting('People can invite others', view.guests, 'Anyone invited may invite more people and bring guests of their own. Off, only the hosts add to the list.',
    on => guests.setGuestsInvite(e, on), on => (on ? 'People can invite others now.' : 'Only the hosts add to the list now.'));
  setting('Send new guests their invitation', view.autoInvite, 'Whoever comes onto the list later - a family joining a class, someone a rule now takes in - is sent the invitation as they come. Off, they wait in Pending for you to send.',
    on => guests.setAutoInvite(e, on), on => (on ? 'New guests are sent the invitation as they come.' : 'New guests wait in Pending for you to send.'));
  setting('Notify me when people respond', view.notifyMe, 'An email to you as each answer comes in.',
    on => guests.setNotifyMe(e, view, on), on => (on ? 'You’ll get an email as answers come in' : 'No more emails about answers'));
  return box;
}

function guestsHead(e, view, refresh) {
  const unsent = view.list.filter(r => r.pending && r.email).length;
  const head = el('div', 'guests-head');
  const headMark = el('div', 'guests-head-mark');
  headMark.append(svg('groups'));
  const headWords = el('div', 'guests-head-words');
  headWords.append(el('div', 'guests-title', 'Guest list'));
  const subtitle = !view.sent
    ? (view.list.length ? '' : 'Nobody on the list yet.')
    : unsent ? `${unsent} added since the invites went out, not sent yet.` : 'Invites are out.';
  if (subtitle) {
    headWords.append(el('div', 'guests-subtitle', subtitle));
  }
  head.append(headMark, headWords, iconButton('gear', 'Guest List Settings', 'guests-settings-button', () => openGuestSettings(e, view, refresh)));
  if (!view.list.length) {
    return head;
  }
  const tools = el('div', 'guests-tools');
  tools.append(button('See table', 'menu', 'button button-secondary', () => openTable(e, view, refresh)));
  const menu = sendMenu(e, view, refresh);
  if (menu) {
    tools.append(menu);
  }
  head.append(tools);
  return head;
}

function sendMenu(e, view, refresh) {
  const waiting = view.list.filter(r => r.invited && r.sent && !r.answer).length;
  const first = eventDates(e)[0];
  const day = weekdayLong(first) + ', ' + parseDate(first).toLocaleDateString('en-US', {month: 'long', day: 'numeric'});
  const when = e.allDay ? day : `${day}, ${timeLine(e)}`;
  const pending = view.list.filter(r => r.pending && r.email).length;
  const pendingItem = {icon: 'send', words: `Pending invites · ${pending}`, run: () => openPending(e, view, refresh)};
  if (!view.sent) {
    return pending ? menuButton('Send', 'mail', [pendingItem]) : null;
  }
  return menuButton('Send', 'mail', [
    ...(pending ? [pendingItem] : []),
    {icon: 'clock', words: `RSVP reminder · ${waiting}`, run: () => openMessage(e, view, refresh, {
      title: 'Send RSVP reminder', to: ['none'], subject: 'Reminder: you’re invited!', attach: true,
      message: `We haven’t heard back from you yet - please let us know if you can make it on ${when}.`,
    })},
    {icon: 'calcheck', words: 'Event reminder', run: () => openMessage(e, view, refresh, {
      title: 'Send event reminder', to: ['yes', 'none'], subject: `Reminder: ${day}`, attach: true,
      message: `Just a reminder that we’re on for ${when}${e.location ? ' at ' + e.location : ''}. See you there!`,
    })},
    {icon: 'chat', words: 'Message', run: () => openMessage(e, view, refresh)},
  ]);
}

function guestCounts(e, view, refresh) {
  const c = view.counts;
  const listOf = keep => () => familiesOf(view.list).flat().filter(keep).map(r => guestPlainRow(e, view, r, refresh));
  const unsent = r => r.pending && r.email && !r.answer;
  const sent = r => !unsent(r);
  const waiting = r => !r.answer && sent(r);
  const items = [
    {icon: 'mail', label: 'Invited', count: view.list.filter(sent).length, tone: 'all', expand: listOf(sent)},
    {icon: 'check', label: 'Yes', count: c.yes, tone: 'yes', expand: listOf(r => r.answer === 'yes')},
    {icon: 'ban', label: 'No', count: c.no, tone: 'no', expand: listOf(r => r.answer === 'no')},
    {icon: 'reply', label: 'No reply yet', count: view.list.filter(waiting).length, tone: 'waiting', expand: listOf(waiting)},
    {icon: 'clock', label: 'Unsent (Pending)', count: view.list.filter(unsent).length, tone: 'pending', expand: listOf(unsent)},
  ];
  if (view.sent) {
    items.push({icon: 'eye', label: 'Opened', count: view.list.filter(r => r.opened).length, tone: 'opened', expand: listOf(r => r.opened)});
  }
  return countList(items);
}

function guestPlainRow(e, view, r, refresh) {
  const bits = [r.guestOf ? `Guest of ${r.guestOfName}` : r.line, r.pending ? 'Not sent' : r.opened ? 'Opened' : ''].filter(Boolean);
  const after = [];
  if (r.warning) {
    const warn = el('span', 'guests-plain-warn');
    warn.title = r.warningWords;
    warn.append(svg('info'));
    after.push(warn);
  }
  const said = el('span', 'invite-said-mark guests-plain-mark is-' + (r.answer || 'none'));
  said.title = r.answer ? answerWords[r.answer] : 'No response';
  said.append(svg(answerIcon(r.answer)));
  after.push(said);
  return personRow(r, {
    className: 'guests-plain' + (r.guestOf ? ' is-guest' : ''),
    onClick: () => openGuestCard(e, r, view, refresh),
    lines: [bits.join(' · ')],
    after,
    gradeColors: state.model.gradeColors,
  });
}

export async function openEditor(e, view, refresh) {
  if (!e.can.edit && !e.can.correct) {
    return;
  }
  const {eventForm} = await import('./eventform.js');
  let shut = null;
  const box = el('div', 'editor');
  box.append(eventForm({edit: e, override: e.can.correct && e.source !== 'sheet', more: [], onDone: async (ids, changed) => {
    shut();
    await refresh();
    if (changed && changed.length) {
      offerUpdate(e, changed);
    }
  }}));
  if (view && view.host && e.source === 'sheet' && !e.cancelled) {
    const foot = el('div', 'editor-danger');
    foot.append(view.sent
      ? button('Cancel event', 'close', 'link-button danger', () => {
        shut();
        openCancel(e, view, refresh);
      })
      : button('Delete event', 'trash', 'link-button danger', () => deleteInvitation(e)));
    box.append(foot);
  }
  shut = popup('Edit ' + e.title, box, {wide: true}).shut;
}

export async function offerUpdate(e, changed) {
  let view;
  try {
    view = await fetchInvites(e);
  } catch (err) {
    toast('Couldn’t check who has the invitation: ' + err.message);
    return;
  }
  if (!view.host || !view.sent) {
    return;
  }
  const people = view.list.filter(r => r.sent && r.email && r.answer !== 'no');
  if (!people.length) {
    return;
  }
  const nos = view.list.filter(r => r.sent && r.email && r.answer === 'no').length;
  const words = {title: 'the title', start: 'the date or time', end: 'the end time', location: 'the location'};
  const what = [...new Set(changed.map(k => words[k]).filter(Boolean))].join(', ').replace(/, ([^,]*)$/, ' and $1');
  if (!what) {
    return;
  }
  const box = el('div');
  box.append(el('p', 'hint', `You changed ${what}. ${people.length} ${people.length === 1 ? 'person has' : 'people have'} the invitation already - send it again with the new details? Their calendar invite is replaced with the new one.${nos ? ` The ${nos === 1 ? 'one who' : nos + ' who'} said no ${nos === 1 ? 'is' : 'are'} not sent it.` : ''}`));
  const actions = el('div', 'modal-actions');
  let shut = null;
  actions.append(button(`Send the update to ${people.length}`, 'mail', 'button', async () => {
    try {
      await guests.sendUpdate(e);
      longToast(people.length === 1 ? 'The update is on its way' : `${people.length} updates are on their way`);
      shut();
    } catch (err) {
      toast(err.message);
    }
  }), button('Not now', null, 'button button-secondary', () => shut()));
  box.append(actions);
  shut = popup('Send an updated invitation?', box).shut;
}

export function inviteHostCall(e, view, refresh) {
  const card = el('div', 'guests-start');
  card.append(svg('people'));
  const words = el('div', 'guests-start-words');
  words.append(el('div', 'guests-start-title', 'Invite people'), el('div', 'guests-start-lead', 'Build a guest list from the directory, a classroom, your lists or anyone by email, then send everyone the invitation with a calendar invite attached.'));
  card.append(words, button('Add People', 'plus', 'button', () => openPicker(e, view, refresh)));
  return card;
}
