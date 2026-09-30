import {state, me, allows, answer, isParty, eventDates, weekdayLong, parseDate, timeLine} from './state.js';
import {el, svg, button, iconButton, toast, longToast} from '/elements.js';
import {popup} from '/modal.js';
import {api} from '/api.js';
import {query, act, create, remove as removeResource} from '/data.js';
import {checkbox} from '/form.js';
import {addressSuggest} from '/address.js';
import {createPersonPicker} from '/picker.js';
import {uploadImage} from './imagecontrol.js';
import {answerWords, answerIcon, answerFor, firstName, ticketWords, stamp, pickerPeople, setRuleOptions, setNames, groupRule, rules} from './inviteparts.js';
import {openGuestForm, openGuestCard, openPending, sendInvites, deleteInvitation, openCancel} from './guestpopups.js';
import {listFilters, openTable, openMessage} from './guesttable.js';
import {openPicker, guestAdders} from './addpeople.js';
import {personTile, personCard, peopleRow, andList} from '/people.js';
import {personRow} from '/personrow.js';
import {countList} from '/countlist.js';
import {openPersonCard} from '/personcard.js';
import {memberAdders, membersCard} from '/members.js';
import {directory} from '/directory.js';
import {tabbedFields} from '/tabs.js';

export async function startParty(e) {
  await act('events', e.id, 'start');
  const read = await query('/api/events/' + encodeURIComponent(e.id) + '?include=guest-list.invite-groups');
  const key = (isParty(e) ? 'party:' : 'activity:') + e.linkedId;
  const group = read.all('invite-groups').find(g => g.rule.tags.includes(key));
  return {added: group.count};
}

export async function fetchInvites(e) {
  const read = await query('/api/events/' + encodeURIComponent(e.id) + '?include=guest-list');
  const view = read.follow(read.get(read.result), 'guest-list');
  if (view.can.opened) {
    act('guest-lists', view.id, 'opened').catch(err => toast(err.message));
  }
  return view;
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

function memberRow(e, r, refresh) {
  const row = el('div', 'rsvp-panel-person');
  row.append(personRow(r, {
    className: 'rsvp-person',
    lines: [r.guestOf ? `Guest of ${r.guestOfName}` : r.invitedBy ? `Invited by ${r.invitedBy}${r.invitedAt ? ' on ' + stamp(r.invitedAt) : ''}` : ''],
    gradeColors: state.model.gradeColors,
  }), bigChoices(e, r, refresh));
  if (r.guestOf && r.mine) {
    const remove = el('button', 'invite-remove rsvp-person-remove');
    remove.type = 'button';
    remove.title = 'Remove this guest';
    remove.append(svg('close'));
    remove.addEventListener('click', async () => {
      try {
        await act('events', e.id, 'uninvite', {email: r.key});
        toast(`${r.name} removed`);
        refresh();
      } catch (err) {
        toast(err.message);
      }
    });
    row.append(remove);
  }
  return row;
}

export function familyBand(e, view, refresh) {
  const box = el('div', 'rsvp-panel');
  const self = view.mine.find(r => r.key === me().email && !r.guestOf);
  const others = view.mine.filter(r => r !== self);
  const asked = view.mine.some(r => r.invited);
  const hosts = view.hosts.filter(h => h.email !== me().email).map(h => h.name).filter(Boolean);
  const by = asked && hosts.length ? `Invited by ${hosts.join(' and ')}. ` : '';
  const top = el('div', 'rsvp-panel-top');
  const icon = el('div', 'guests-table-mark');
  icon.append(svg('person'));
  const words = el('div', 'guests-start-words');
  words.append(el('div', 'guests-title', self ? 'Your RSVP' : 'Your family\u2019s RSVP'), el('div', 'guests-start-lead', by + (self ? 'Will you be attending?' : 'Choose who from your family will be attending.')));
  top.append(icon, words);
  if (self) {
    top.append(bigChoices(e, self, refresh));
  }
  box.append(top);
  for (const r of others) {
    box.append(memberRow(e, r, refresh));
  }
  const foot = el('div', 'rsvp-panel-foot');
  const of = self ? me().email : (view.mine.find(r => r.mine && !r.guestOf) || {}).key;
  if (of && (view.guests || view.host)) {
    foot.append(button('Add guests', 'plus', 'rsvp-clear rsvp-add', () => openGuestForm(e, of, refresh)));
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
    const paintLine = () => {
      line.replaceChildren();
      const said = el(r.mine ? 'button' : 'span', 'invite-said rsvp-row-said is-' + (r.answer || 'none'));
      const mark = el('span', 'invite-said-mark');
      mark.append(svg(answerIcon(r.answer)));
      said.append(mark, el('strong', '', r.key === me().email ? 'You' : firstName(r)), el('span', 'rsvp-row-word', r.answer ? answerWords[r.answer] : 'No response'));
      if (r.mine) {
        said.type = 'button';
        said.title = 'Change the answer';
        said.append(svg('chevron-down'));
        line.append(dropMenu(said, answerItems(e, r, refresh, 'Clear'), true));
      } else {
        line.append(said);
      }
      if (r.guestOf && r.mine) {
        const remove = el('button', 'invite-remove rsvp-row-remove');
        remove.type = 'button';
        remove.title = 'Remove this guest';
        remove.append(svg('close'));
        remove.addEventListener('click', async () => {
          try {
            await act('events', e.id, 'uninvite', {email: r.key});
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
  if (of && (view.guests || view.host)) {
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

export function comingCard(e, view, refresh) {
  const card = el('section', 'rsvps-card coming-section');
  if (view.host) {
    const unsent = view.list.filter(r => r.invited && !r.sent && r.email);
    if (unsent.length) {
      const pending = el('div', 'rsvps-pending');
      const mark = el('span', 'rsvps-pending-mark');
      mark.append(svg('mail'));
      const words = el('div', 'rsvps-pending-words');
      const sub = el('div', 'rsvps-pending-sub');
      sub.append(el('span', '', `${unsent.length} ${unsent.length === 1 ? 'invitation' : 'invitations'} not sent yet`), button('More info', 'info', 'link-button rsvps-pending-info', () => openPending(e, view, refresh)));
      words.append(el('div', 'rsvps-pending-title', 'Pending'), sub);
      const send = button('Send Now', 'mail', 'button rsvps-pending-send', () => sendInvites(e, 'new', unsent.length, `Send the invitation to ${unsent.length} ${unsent.length === 1 ? 'person' : 'people'} who have not had it yet? Each gets an email with the calendar invite; a student's goes to them and their parents.`, refresh));
      pending.append(mark, words, send);
      card.append(pending);
    }
  }
  const rows = (view.host ? view.list : view.coming).filter(r => r.answer === 'yes' || r.answer === 'maybe' || (view.host && r.answer === 'no') || (r.invited && !r.answer));
  const head = el('div', 'rsvps-card-head');
  head.append(el('h2', 'section section-swoosh', 'Who\u2019s coming'));
  const imported = e.source === 'google' || e.source === 'pdf';
  card.append(head);
  if (view.host && imported) {
    card.append(el('div', 'side-line coming-privacy', view.listPrivate ? 'Only the hosts see this list.' : 'Everyone who opens the event sees this list.'));
  }
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
    const unsent = r => view.host && r.invited && !r.sent && r.email && !r.answer;
    groups.push(['No response', shown.filter(r => r.invited && !r.answer && !unsent(r)), 'is-waiting']);
    if (view.host) {
      groups.push(['Pending (Unsent)', shown.filter(unsent), 'is-pending']);
    }
    if (!groups.some(([, people]) => people.length)) {
      grid.append(el('div', 'side-line', rows.length ? 'Nobody matches.' : 'Nobody has answered yet.'));
      return;
    }
    for (const [label, people, cls] of groups) {
      if (!people.length) {
        continue;
      }
      grid.append(el('div', 'rsvps-head ' + cls, label));
      const list = el('div', 'person-cards');
      for (const p of people) {
        let mark = null;
        if (view.party && view.host && p.invited) {
          mark = el('span', 'ticket-mark is-' + (p.ticket || 'none'));
          mark.title = ticketWords(p.ticket);
          mark.append(svg(p.ticket === 'ticket' ? 'ticket' : p.ticket === 'free' ? 'gift' : p.ticket === 'waitlist' ? 'clock' : 'close'));
        }
        list.append(personCard(p, {
          onClick: () => openGuestCard(e, p, view, refresh),
          line: p.guestOf ? `Guest of ${p.guestOfName}` : p.line,
          mine: view.mine.some(m => m.key === p.key),
          corner: mark,
          gradeColors: state.model.gradeColors,
        }));
      }
      grid.append(list);
    }
  };
  if (rows.length > 1) {
    card.append(listFilters(rows, view, paint, {rsvp: false}));
  }
  paint(rows);
  card.append(grid);
  return card;
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
  const s = view.settings || {};
  const cohosts = new Set(s.hosts || []);
  const self = me().email;
  const tiles = [];
  for (const h of view.hosts) {
    const tile = personTile(h, {
      onClick: () => openPersonCard(h),
      title: [h.name, h.line].filter(Boolean).join(' \u00b7 '),
      gradeColors: state.model.gradeColors,
    });
    const own = h.email === self;
    const poster = !own && allows('when.act-as-host') && h.email === view.poster;
    if (view.host && (own ? cohosts.has(self) || view.poster === self : cohosts.has(h.email) || poster)) {
      const x = el('button', 'hosts-card-remove');
      x.type = 'button';
      x.title = own ? 'Step down as host' : poster ? `Step ${h.name} down as host` : `Take ${h.name} off as a co-host`;
      x.textContent = '\u00d7';
      x.addEventListener('click', async ev => {
        ev.stopPropagation();
        const lose = view.poster === self ? 'edit it or run its guest list' : 'run its guest list';
        const alone = !view.hosts.some(o => o.email !== self);
        const others = view.hosts.filter(o => o.email !== h.email).length;
        const ask = own
          ? `Step down as host of ${e.title}? You won't be able to ${lose} any more${alone ? ', and it will have no host - an admin can still change it' : ''}.`
          : poster
            ? `Step ${h.name} down as host of ${e.title}? They added it and stay named as the one who shared it, but can no longer edit it or run its guest list${others ? '' : ', and it will have no host'}.`
            : `Take ${h.name} off as a co-host?`;
        if (!confirm(ask)) {
          return;
        }
        try {
          if (own) {
            await act('events', e.id, 'step-down', {});
            toast('You no longer host this event.');
          } else if (poster) {
            await act('events', e.id, 'step-down', {email: h.email});
            toast(`${h.name} no longer hosts this event.`);
          } else {
            await act('events', e.id, 'settings', {hosts: [...cohosts].filter(x => x !== h.email)});
          }
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
  if (view.host) {
    const tools = el('div', 'hosts-tools');
    tools.append(button('Add co-host', 'plus', 'link-button', () => openAddHost(e, view, refresh)));
    const hidden = Boolean(view.hostsHidden);
    tools.append(button(hidden ? 'Show hosts' : 'Hide hosts', hidden ? 'eye' : 'eye-off', 'link-button', async () => {
      try {
        await act('events', e.id, 'settings', {hideHosts: !hidden});
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
  const picker = createPersonPicker(mount, {people: pickerPeople(p => !view.hosts.some(h => h.email === p.email))});
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
      await act('events', e.id, 'settings', {hosts: [...((view.settings || {}).hosts || []), email]});
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
    const b = el('button', 'hero-image-menu-item');
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
        await act('events', e.id, 'settings', {flyer: image});
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

export function addFlyerLink(e, view, refresh) {
  if (!view.host || view.flyer) {
    return null;
  }
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
      await act('events', e.id, 'settings', {flyer: made.name});
      toast('Flyer saved');
      refresh();
    } catch (err) {
      toast(err.message);
    }
  });
  const link = el('label', 'side-add-flyer');
  link.append(svg('plus'), el('span', '', 'Add a flyer'), file);
  return link;
}

export function guestListSection(e, view, refresh) {
  const section = el('section', 'side-card guests-section');
  section.append(guestsHead(e, view, refresh), guestCounts(e, view, refresh));
  if (!view.list.length) {
    section.append(el('p', 'guests-note', 'Add people from the directory, a classroom, one of your lists' + (view.party ? ', the ticket holders' : '') + ', or by email - then send the invitation.'));
    return section;
  }
  if (view.sent) {
    const foot = el('div', 'guests-cancel');
    foot.append(view.linked
      ? button('Delete invite', 'trash', 'link-button danger', () => deleteInvitation(e, view))
      : button('Cancel event', 'close', 'link-button danger', () => openCancel(e, view, refresh)));
    section.append(foot);
  }
  return section;
}


export async function openGuestSettings(e, view, refresh) {
  let people = null;
  try {
    const [options, dir] = await Promise.all([api('GET', '/api/when/invites/options'), directory()]);
    setRuleOptions(options);
    people = new Map(dir.result.map(dir.get).map(p => [p.email, p]));
    setNames(people.values());
  } catch (err) {
    toast('Couldn’t load the guest list’s rules: ' + err.message);
    return;
  }
  const guests = el('div');
  const again = async () => {
    refresh();
    try {
      view = await fetchInvites(e);
    } catch (err) {
      toast(err.message);
      return;
    }
    paintGuests(e, view, people, guests, again);
  };
  paintGuests(e, view, people, guests, again);
  const box = el('div', 'guest-settings');
  box.append(tabbedFields([{label: 'Guests', fields: [guests]}, {label: 'Settings', fields: [settingsPanel(e, view, refresh)]}]));
  popup('Guest List Settings', box, {wide: true});
}

function autoToggle(rule, again) {
  const auto = el('label', 'guests-group-auto');
  const box = el('input');
  box.type = 'checkbox';
  box.checked = rule.auto !== false;
  box.addEventListener('change', async () => {
    rule.auto = box.checked;
    if (!rule.id) {
      return;
    }
    try {
      await act('invite-groups', rule.id, 'edit', {auto: box.checked});
      toast(box.checked ? (rule.sent ? 'Auto-invite on: newcomers are sent their invitation' : 'Auto-invite on: newcomers are sent theirs once you have sent this rule its invites') : 'Auto-invite off: newcomers wait in Pending for you to send');
      again();
    } catch (err) {
      toast(err.message);
      box.checked = !box.checked;
      rule.auto = box.checked;
    }
  });
  const info = el('span', 'guests-group-info');
  info.title = 'Whoever comes to match this rule later goes on the list; with Auto-invite on they are sent the invitation too, once you have sent this rule its invites.';
  info.append(svg('info'));
  auto.append(box, el('span', '', 'Auto-invite'), info);
  return auto;
}

function ruleCells(r) {
  return {kind: r.kind, roles: r.roles, search: r.search, classrooms: r.classrooms, grades: r.grades, tags: r.tags, family: r.family};
}

function guestView(r, items, people) {
  const reasons = r.guestOf ? [{guestOf: r.guestOfName}]
    : (r.via || '').startsWith('group:') ? [{rule: items.findIndex(i => 'group:' + i.id === r.via)}]
      : r.via === 'invited' && r.invitedBy ? [{invitedBy: r.invitedBy}] : [{added: true}];
  const p = people.get(r.email);
  if (!p) {
    return {email: r.email, name: r.name, key: r.key, words: 'Outside the directory', outside: true, reasons};
  }
  return {email: p.email, name: p.fullName, key: r.key, photoUrl: p.heroPhotoUrl, words: p.words, grade: p.isStudent ? p.grade : '', reasons};
}

function paintGuests(e, view, people, panel, again) {
  const items = (view.groups || []).map(g => ({...groupRule(g), roles: [...g.rule.roles], classrooms: [...g.rule.classrooms], grades: [...g.rule.grades], tags: [...g.rule.tags], family: [...g.rule.family], id: g.id, count: g.count, auto: g.auto, sent: g.sent}));
  const ruleCard = rules.rulesCard({
    hint: 'Someone is on the guest list if any include rule matches them and no exclude rule does, or if you or a guest added them yourselves; a rule matches only if every choice in it holds. Whoever comes to match an include rule later goes on too.',
    list: () => items,
    empty: 'No rules yet: everyone on the list was added one at a time.',
    onAdd: rule => items.push(rule),
    onChange: () => {},
    onRemove: async rule => {
      if (!rule.id) {
        items.splice(items.indexOf(rule), 1);
        ruleCard.list.render();
        return;
      }
      const ask = rule.kind === 'exclude' ? 'Remove this exclude rule? Whoever it left out goes back on the list if an include rule matches them.' : 'Remove this rule? Anyone already sent an invitation will stay, but pending guests will be removed.';
      if (!confirm(ask)) {
        return;
      }
      try {
        await removeResource('invite-groups', rule.id);
        toast('Rule removed');
        again();
      } catch (err) {
        toast(err.message);
      }
    },
    onDone: async rule => {
      try {
        if (rule.id) {
          await act('invite-groups', rule.id, 'edit', {rule: ruleCells(rule)});
          toast('Rule saved');
        } else {
          await create('invite-groups', {id: e.id, rule: ruleCells(rule), auto: rule.auto !== false});
          toast(rule.kind === 'exclude' ? 'Exclude rule added' : 'Rule added');
        }
        again();
      } catch (err) {
        toast(err.message);
      }
    },
    count: rule => (rule.id ? rule.count : undefined),
    extra: rule => (rule.kind === 'include' ? autoToggle(rule, again) : null),
  });
  const adders = guestAdders(e, view, words => {
    longToast(words);
    again();
  });
  const takeOff = async m => {
    try {
      await act('events', e.id, 'uninvite', {email: m.key});
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
    phrase: rules.personWords,
    chip: () => null,
    onExclude: takeOff,
    onRemove: takeOff,
    gradeColors: state.model.gradeColors,
  });
  const byRule = r => (r.via || '').startsWith('group:');
  const invited = view.list.filter(r => r.invited).sort((a, b) => byRule(a) - byRule(b) || (b.invitedAt || '').localeCompare(a.invitedAt || ''));
  members.show({members: invited.map(r => guestView(r, items, people)), rules: items});
  panel.replaceChildren(ruleCard.card, members.card);
}

function settingsPanel(e, view, refresh) {
  const box = el('div', 'guests-settings');
  const setting = (label, on, hint, body, words) => {
    const toggle = checkbox(label, on, hint);
    toggle.input.addEventListener('change', async () => {
      const next = toggle.input.checked;
      try {
        await act('events', e.id, 'settings', body(next));
        toast(words(next));
        refresh();
      } catch (err) {
        toast(err.message);
        toggle.input.checked = !next;
      }
    });
    box.append(toggle.wrap);
  };
  setting('Show the guest list to everyone', !view.listPrivate, 'Everyone who can open the event sees who said yes or maybe, and who has not answered. Only the hosts see who said no.',
    on => ({publicList: on}), on => (on ? 'Everyone who opens the event sees who is coming now.' : 'Only the hosts see who is coming now.'));
  setting('People can invite others', view.guests, 'Anyone invited may invite more people and bring guests of their own. Off, only the hosts add to the list.',
    on => ({guests: on}), on => (on ? 'People can invite others now.' : 'Only the hosts add to the list now.'));
  setting('Notify me when people respond', view.notifyMe, 'An email to you as each answer comes in.',
    on => ({notifyMe: on}), on => (on ? 'You’ll get an email as answers come in' : 'No more emails about answers'));
  return box;
}

function guestsHead(e, view, refresh) {
  const unsent = view.list.filter(r => r.invited && !r.sent).length;
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
  const pending = view.list.filter(r => r.invited && !r.sent && r.email).length;
  const pendingItem = {icon: 'send', words: `Pending invites \u00b7 ${pending}`, run: () => openPending(e, view, refresh)};
  if (!view.sent) {
    return pending ? menuButton('Send', 'mail', [pendingItem]) : null;
  }
  return menuButton('Send', 'mail', [
    ...(pending ? [pendingItem] : []),
    {icon: 'clock', words: `RSVP reminder \u00b7 ${waiting}`, run: () => openMessage(e, view, refresh, {
      title: 'Send RSVP reminder', to: ['none'], subject: 'Reminder: you\u2019re invited!', attach: true,
      message: `We haven\u2019t heard back from you yet - please let us know if you can make it on ${when}.`,
    })},
    {icon: 'calcheck', words: 'Event reminder', run: () => openMessage(e, view, refresh, {
      title: 'Send event reminder', to: ['yes', 'maybe', 'none'], subject: `Reminder: ${day}`, attach: true,
      message: `Just a reminder that we\u2019re on for ${when}${e.location ? ' at ' + e.location : ''}. See you there!`,
    })},
    {icon: 'chat', words: 'Message', run: () => openMessage(e, view, refresh)},
  ]);
}

function guestCounts(e, view, refresh) {
  const c = view.counts;
  const listOf = keep => () => householdsOf(view.list).flat().filter(keep).map(r => guestPlainRow(e, view, r, refresh));
  const unsent = r => r.invited && !r.sent && r.email && !r.answer;
  const sent = r => !unsent(r);
  const waiting = r => !r.answer && sent(r);
  const items = [
    {icon: 'mail', label: 'Invited', count: view.list.filter(sent).length, tone: 'all', expand: listOf(sent)},
    {icon: 'check', label: 'Yes', count: c.yes, tone: 'yes', expand: listOf(r => r.answer === 'yes')},
    {icon: 'help', label: 'Maybe', count: c.maybe, tone: 'maybe', expand: listOf(r => r.answer === 'maybe')},
    {icon: 'ban', label: 'No', count: c.no, tone: 'no', expand: listOf(r => r.answer === 'no')},
    {icon: 'reply', label: 'No reply yet', count: view.list.filter(waiting).length, tone: 'waiting', expand: listOf(waiting)},
    {icon: 'clock', label: 'Unsent (Pending)', count: view.list.filter(unsent).length, tone: 'pending', expand: listOf(unsent)},
  ];
  if (view.ticketHolders) {
    items.push(ticketHoldersItem(e, view, refresh));
  }
  if (view.sent) {
    items.push({icon: 'eye', label: 'Opened', count: view.list.filter(r => r.opened).length, tone: 'opened', expand: listOf(r => r.opened)});
  }
  return countList(items);
}

function ticketHoldersItem(e, view, refresh) {
  const holders = view.ticketHolders;
  return {icon: 'ticket', label: 'Ticket holders', count: holders.length, tone: 'ticket', expand: () => householdsOf(holders).flat().map(r => guestPlainRow(e, view, r, refresh))};
}

export function ticketHoldersSection(e, view, refresh) {
  if (!view.ticketHolders) {
    return null;
  }
  const section = el('section', 'side-card guests-section');
  const head = el('div', 'guests-head');
  const mark = el('div', 'guests-head-mark');
  mark.append(svg('ticket'));
  const words = el('div', 'guests-head-words');
  words.append(el('div', 'guests-title', 'Tickets'), el('div', 'guests-subtitle', 'Who holds a ticket on Celebrate.'));
  head.append(mark, words);
  section.append(head, countList([ticketHoldersItem(e, view, refresh)]));
  return section;
}

function householdsOf(rows) {
  const households = new Map();
  for (const r of rows) {
    const key = r.household || r.key;
    if (!households.has(key)) {
      households.set(key, []);
    }
    households.get(key).push(r);
  }
  return [...households.values()].sort((a, b) => (a[0].name || '').localeCompare(b[0].name || ''));
}

function guestPlainRow(e, view, r, refresh) {
  const bits = [r.guestOf ? `Guest of ${r.guestOfName}` : r.line, r.invited && !r.sent ? 'Not sent' : r.opened ? 'Opened' : ''].filter(Boolean);
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
    lines: [bits.join(' \u00b7 ')],
    after,
    gradeColors: state.model.gradeColors,
  });
}

export function openSettings(e, view, refresh) {
  let shut = null;
  const form = settingsForm(e, view, refresh, () => shut());
  shut = popup('Edit the invitation', form, {wide: Boolean(view.linked)}).shut;
}

export function settingsForm(e, view, refresh, shut, part = 'invitation') {
  const s = view.settings || {audience: 'both', guests: true, message: '', hosts: []};
  const form = el('form', 'admin-form');
  const invitation = part === 'invitation';
  const field = (label, input, note) => {
    const wrap = el('div', 'field');
    wrap.append(el('span', '', label), input);
    if (note) {
      wrap.append(el('small', '', note));
    }
    return wrap;
  };
  let details = null;
  if (view.linked && invitation) {
    const text = (value, placeholder) => {
      const input = el('input');
      input.type = 'text';
      input.value = value || '';
      input.placeholder = placeholder || '';
      return input;
    };
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
    addressSuggest(location);
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
  const message = el('textarea');
  message.rows = 4;
  message.value = s.message || '';
  message.placeholder = 'A few words on the invitation - what to bring, where to park\u2026';
  const messageField = field('Email invitation text', message, 'Your own words on the invitation email, under the event and before its description. Leave it blank and the email carries the event\u2019s title, date, place and description, with your name as the host.');
  if (!invitation) {
    form.append(el('p', 'hint', 'Everyone invited gets the same email: who sent it, the event, its picture, and the way to RSVP. Edit the text below to add a note of your own.'));
    form.append(messageField);
  }

  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  actions.append(submit, status);
  form.append(actions);
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    submit.disabled = true;
    try {
      const own = details ? details() : {};
      await act('events', e.id, 'settings', {...(invitation ? {} : {message: message.value}), ...own});
      toast('Saved');
      shut();
      await refresh();
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
  return form;
}

export async function openEditor(e, view, refresh, {tab = 'event'} = {}) {
  const {eventForm} = await import('./eventform.js');
  const host = Boolean(view && view.host);
  const own = e.can.edit;
  const imported = e.can.correct;
  let shut = null;
  const box = el('div', 'editor');
  const panels = {};
  if (own || imported) {
    const more = host ? [{label: 'Message', panel: settingsForm(e, view, refresh, () => shut(), 'email')}] : [];
    panels.event = eventForm({edit: e, override: imported, more, onDone: async (ids, changed) => {
      shut();
      await refresh();
      if (changed && changed.length) {
        offerUpdate(e, changed);
      }
    }});
  } else if (host) {
    if (view.linked) {
      panels.invitation = settingsForm(e, view, refresh, () => shut(), 'invitation');
    }
    panels.email = settingsForm(e, view, refresh, () => shut(), 'email');
  }
  const keys = Object.keys(panels);
  if (keys.length > 1) {
    const tabs = el('div', 'tabs');
    let active = keys.includes(tab) ? tab : keys[0];
    const paint = () => {
      for (const [key, panel] of Object.entries(panels)) {
        panel.hidden = key !== active;
      }
      for (const b of tabs.children) {
        b.classList.toggle('is-active', b.dataset.key === active);
      }
    };
    for (const [key, label] of [['event', 'Event'], ['invitation', 'Invitation'], ['email', 'Email Invitation']]) {
      if (!panels[key]) {
        continue;
      }
      const b = el('button', 'tab-button', label);
      b.type = 'button';
      b.dataset.key = key;
      b.addEventListener('click', () => {
        active = key;
        paint();
      });
      tabs.append(b);
    }
    box.append(tabs);
    for (const panel of Object.values(panels)) {
      box.append(panel);
    }
    paint();
  } else {
    box.append(...Object.values(panels));
  }
  if (view && view.host && e.source === 'sheet' && !e.cancelled) {
    const foot = el('div', 'editor-danger');
    foot.append(view.sent
      ? button('Cancel event', 'close', 'link-button danger', () => {
        shut();
        openCancel(e, view, refresh);
      })
      : button('Delete event', 'trash', 'link-button danger', () => deleteInvitation(e, view)));
    box.append(foot);
  }
  if (view && view.host && isParty(e) && view.settings) {
    const foot = el('div', 'editor-danger');
    foot.append(button('Delete invite', 'trash', 'link-button danger', () => deleteInvitation(e, view)));
    box.append(foot);
  }
  if (!Object.keys(panels).length && !box.childElementCount) {
    return;
  }
  shut = popup(own || imported ? 'Edit ' + e.title : 'Edit the invitation', box, {wide: true}).shut;
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
  const people = view.list.filter(r => r.invited && r.sent && r.email && r.answer !== 'no');
  if (!people.length) {
    return;
  }
  const nos = view.list.filter(r => r.invited && r.sent && r.email && r.answer === 'no').length;
  const words = {title: 'the title', start: 'the date or time', end: 'the end time', location: 'the location'};
  const what = [...new Set(changed.map(k => words[k]).filter(Boolean))].join(', ').replace(/, ([^,]*)$/, ' and $1');
  const box = el('div');
  box.append(el('p', 'hint', `You changed ${what}. ${people.length} ${people.length === 1 ? 'person has' : 'people have'} the invitation already - send it again with the new details? Their calendar invite is replaced with the new one.${nos ? ` The ${nos === 1 ? 'one who' : nos + ' who'} said no ${nos === 1 ? 'is' : 'are'} not sent it.` : ''}`));
  const actions = el('div', 'modal-actions');
  let shut = null;
  actions.append(button(`Send the update to ${people.length}`, 'mail', 'button', async () => {
    try {
      await act('events', e.id, 'send', {to: 'sent', update: true});
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
  words.append(el('div', 'guests-start-title', e.link ? 'Invite your guests' : 'Invite people'), el('div', 'guests-start-lead', isParty(e) ? 'Ask the ticket holders to confirm they are coming - or invite anyone else - and see who has answered beside who has a ticket.' : e.link ? 'Ask the volunteers to confirm they are coming - or invite anyone else - and see who has answered beside the sign-ups on HCA-Team.' : 'Build a guest list from the directory, a classroom, your lists or anyone by email, then send everyone the invitation with a calendar invite attached.'));
  card.append(words);
  if (e.link) {
    const party = isParty(e);
    card.append(button(party ? 'Add the Ticket Holders' : 'Invite the volunteers', party ? 'ticket' : 'people', 'button', async () => {
      try {
        const made = await startParty(e);
        const who = party ? ['ticket holders', 'the tickets'] : ['volunteers', 'the sign-ups'];
        longToast(made.added ? `${made.added} ${who[0]} on the list - it follows ${who[1]} from here.` : `The list follows ${who[1]} from here.`);
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  card.append(button(isParty(e) ? 'Add Others' : 'Add People', 'plus', 'button' + (isParty(e) ? ' button-secondary' : ''), () => openPicker(e, view, refresh)));
  return card;
}
