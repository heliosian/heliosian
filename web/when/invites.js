import {me, isAdmin, postedAndHosting, answer, isParty, eventDates, weekdayLong, parseDate, timeLine} from './state.js';
import {el, svg, button, toast, longToast, copyText} from '/elements.js';
import {popup} from '/modal.js';
import {whoLink} from '/appswitch.js';
import {api} from '/api.js';
import {checkbox} from '/form.js';
import {addressSuggest} from '/address.js';
import {createPersonPicker} from '/picker.js';
import {uploadImage} from './imagecontrol.js';
import {answerWords, firstName, answerButtons, face, ticketWords, stamp, answeredWords, pickerPeople, ruleOptions, setRuleOptions, groupWords} from './inviteparts.js';
import {openGuestForm, openGuestCard, warningChip, openPending, sendInvites, deleteInvitation, openCancel} from './guestpopups.js';
import {listFilters, openTable, openMessage} from './guesttable.js';
import {openPicker} from './addpeople.js';

export function startParty(e) {
  return api('POST', '/api/when/invites/start', {id: e.id});
}

export function fetchInvites(e) {
  return api('GET', '/api/when/invites?id=' + encodeURIComponent(e.id));
}

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
    const tools = el('div', 'invite-tools');
    const paintTools = editing => {
      tools.replaceChildren();
      if (r.answer && !editing) {
        const said = el('span', 'invite-said is-' + r.answer);
        const mark = el('span', 'invite-said-mark');
        mark.append(svg(r.answer === 'yes' ? 'check' : r.answer === 'maybe' ? 'clock' : 'close'));
        said.append(mark, el('span', '', `${r.key === me().email ? 'You' : firstName(r)} said `), el('strong', '', answerWords[r.answer]));
        tools.append(said);
        if (r.mine) {
          tools.append(button('Edit', 'edit', 'link-button fam-edit', () => paintTools(true)));
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
          await api('DELETE', '/api/when/invites/people', {id: e.id, email: r.key});
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
            await api('DELETE', '/api/when/invites/people', {id: e.id, email: r.key});
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
      pending.append(el('div', 'rsvps-pending-title', `Pending \u00b7 ${unsent.length} not sent yet`));
      const row = el('div', 'rsvps-pending-row');
      row.append(button('Send invitation now', 'calendar', 'button button-small', () => sendInvites(e, 'new', `Send the invitation to ${unsent.length} ${unsent.length === 1 ? 'person' : 'people'} who have not had it yet? Each gets an email with the calendar invite; a student's goes to them and their parents.`, refresh)));
      row.append(button('More info', 'info', 'link-button', () => openPending(e, view, refresh)));
      if (!view.sent) {
        row.append(button(view.linked ? 'Delete invite' : 'Delete event', 'trash', 'link-button rsvps-pending-delete', () => deleteInvitation(e, view)));
      }
      pending.append(row);
      card.append(pending);
    }
  }
  const rows = (view.host ? view.list : view.coming).filter(r => r.answer === 'yes' || r.answer === 'maybe' || (view.host && r.answer === 'no') || (r.invited && !r.answer));
  const head = el('div', 'rsvps-card-head');
  head.append(el('h2', 'section section-swoosh', 'Who\u2019s coming'));
  const imported = e.source === 'google' || e.source === 'pdf';
  if (view.host && imported) {
    const open = !view.listPrivate;
    head.append(button(open ? 'Keep to hosts' : 'Show to everyone', open ? 'eye-off' : 'eye', 'link-button', async () => {
      try {
        await api('PUT', '/api/when/invites/settings', {id: e.id, publicList: !open});
        toast(open ? 'Only the hosts see who is coming now.' : 'Everyone who opens the event sees who is coming now.');
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
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
    groups.push(['No response', shown.filter(r => r.invited && !r.answer), 'is-waiting']);
    if (!groups.some(([, people]) => people.length)) {
      grid.append(el('div', 'side-line', rows.length ? 'Nobody matches.' : 'Nobody has answered yet.'));
      return;
    }
    for (const [label, people, cls] of groups) {
      if (!people.length) {
        continue;
      }
      grid.append(el('div', 'rsvps-head ' + cls, label));
      const list = el('div', 'attendee-grid');
      for (const p of people) {
        const tile = el('button', 'attendee');
        tile.type = 'button';
        tile.title = p.name || p.email;
        const photo = face(p, 'attendee-face');
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
  if (view.host) {
    const visible = el('div', 'rsvps-visible');
    visible.append(el('span', '', 'Guest list is visible to everyone who can open the event.'));
    const notify = el('label', 'rsvps-notify');
    const box = el('input');
    box.type = 'checkbox';
    box.checked = Boolean(view.notifyMe);
    box.addEventListener('change', async () => {
      try {
        await api('PUT', '/api/when/invites/settings', {id: e.id, notifyMe: box.checked});
        toast(box.checked ? 'You\u2019ll get an email as answers come in' : 'No more emails about answers');
      } catch (err) {
        toast(err.message);
        box.checked = !box.checked;
      }
    });
    notify.append(box, el('span', '', 'Notify me when people respond'));
    visible.append(notify);
    card.append(visible);
  }
  return card;
}

export function inviteCall(e, view, refresh) {
  if (!view.mayInvite || view.host) {
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
  const row = el('div', 'side-row hosts-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('people'));
  const card = el('div', 'side-row-body');
  card.append(el('div', 'side-title', 'Hosts'));
  const names = view.hosts.map(h => h.name || h.email).filter(Boolean);
  if (names.length) {
    card.append(el('div', 'side-line', names.length > 1 ? names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1] : names[0]));
  }
  if (view.hostsHidden) {
    card.append(el('div', 'side-line hosts-hidden-note', 'Hidden - only the hosts see this.'));
  }
  const s = view.settings || {};
  const cohosts = new Set(s.hosts || []);
  const self = me().email;
  const list = el('div', 'rsvps-grid');
  for (const h of view.hosts) {
    const tile = el(h.email ? 'a' : 'div', 'contact-card');
    if (h.email) {
      tile.href = whoLink(h.email);
    }
    tile.title = [h.name, h.line].filter(Boolean).join(' \u00b7 ');
    tile.append(face(h, 'contact-photo'), el('span', 'contact-name', h.name || h.email));
    const own = h.email === self;
    const poster = !own && isAdmin() && h.email === view.poster;
    if (view.host && (own ? cohosts.has(self) || postedAndHosting(e) : cohosts.has(h.email) || poster)) {
      const x = el('button', 'hosts-card-remove');
      x.type = 'button';
      x.title = own ? 'Step down as host' : poster ? `Step ${h.name} down as host` : `Take ${h.name} off as a co-host`;
      x.textContent = '\u00d7';
      x.addEventListener('click', async ev => {
        ev.preventDefault();
        const lose = postedAndHosting(e) ? 'edit it or run its guest list' : 'run its guest list';
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
            await api('POST', '/api/when/invites/step-down', {id: e.id});
            toast('You no longer host this event.');
          } else if (poster) {
            await api('POST', '/api/when/invites/step-down', {id: e.id, email: h.email});
            toast(`${h.name} no longer hosts this event.`);
          } else {
            await api('PUT', '/api/when/invites/settings', {id: e.id, hosts: [...cohosts].filter(x => x !== h.email)});
          }
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
    const tools = el('div', 'hosts-tools');
    tools.append(button('Add co-host', 'plus', 'link-button', () => openAddHost(e, view, refresh)));
    const hidden = Boolean(view.hostsHidden);
    tools.append(button(hidden ? 'Show hosts' : 'Hide hosts', hidden ? 'eye' : 'eye-off', 'link-button', async () => {
      try {
        await api('PUT', '/api/when/invites/settings', {id: e.id, hideHosts: !hidden});
        toast(hidden ? 'Everyone who opens the event sees its hosts again.' : 'Only the hosts see who hosts this event now.');
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
    card.append(tools);
  }
  row.append(icon, card);
  return row;
}

function openAddHost(e, view, refresh) {
  const form = el('form', 'admin-form');
  form.append(el('p', 'hint', 'A co-host builds and sends the list, reads every answer and hears replies, as you do.'));
  const mount = el('div', 'cohost-picker');
  const picker = createPersonPicker(mount, {people: pickerPeople(e, p => !p.isStudent && !view.hosts.some(h => h.email === p.email))});
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
      await api('PUT', '/api/when/invites/settings', {id: e.id, hosts: [...((view.settings || {}).hosts || []), email]});
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
  const holder = el('div', 'hero-image-menu-holder');
  const toggle = el('button', 'button button-secondary button-small');
  toggle.type = 'button';
  toggle.setAttribute('aria-haspopup', 'menu');
  toggle.append(svg(icon), el('span', '', label), svg('chevron-down'));
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
        await api('PUT', '/api/when/invites/settings', {id: e.id, flyer: image});
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
      await api('PUT', '/api/when/invites/settings', {id: e.id, flyer: made.name});
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

const openGroups = new Set();

const viaWords = {family: 'Family', search: 'Search', classroom: 'Classroom', list: 'List', tickets: 'Tickets', outside: 'By email', guest: 'Guest', link: 'By link', invited: 'Invited'};

function viaLabel(r, view) {
  if (!r.invited) {
    return view.hosts.some(h => h.email === r.key) ? 'Host' : 'By link';
  }
  const [kind, rest] = (r.via || '').split(':');
  if (kind === 'invited' && r.invitedBy) {
    return 'Invited by ' + r.invitedBy;
  }
  if (kind === 'group') {
    const g = (view.groups || []).find(x => x.id === rest);
    return g ? 'Group: ' + groupWords(g) : 'Group';
  }
  if (rest) {
    return rest;
  }
  return viaWords[kind] || '';
}

export function guestListSection(e, view, refresh) {
  const section = el('section', 'side-card guests-section');
  section.append(guestsHead(e, view, refresh), guestStats(e, view, refresh));
  if (!view.list.length) {
    section.append(el('p', 'guests-note', 'Add people from the directory, a classroom, one of your lists' + (view.party ? ', the ticket holders' : '') + ', or by email - then send the invitation.'));
  }
  if ((view.groups || []).length && !ruleOptions) {
    loadGroupNames(section, e, view, refresh);
  }
  if (!view.list.length) {
    return section;
  }
  section.append(guestsTable(e, view, refresh), guestTableButton(e, view, refresh));
  if (view.sent) {
    const foot = el('div', 'guests-cancel');
    foot.append(view.linked
      ? button('Delete invite', 'trash', 'link-button danger', () => deleteInvitation(e, view))
      : button('Cancel event', 'close', 'link-button danger', () => openCancel(e, view, refresh)));
    section.append(foot);
  }
  return section;
}

function loadGroupNames(section, e, view, refresh) {
  api('GET', '/api/when/invites/options').then(options => {
    if (section.isConnected) {
      setRuleOptions(options);
      section.replaceWith(guestListSection(e, view, refresh));
    }
  }).catch(err => toast('Couldn\u2019t load the group names: ' + err.message));
}

function guestsHead(e, view, refresh) {
  const unsent = view.list.filter(r => r.invited && !r.sent).length;
  const head = el('div', 'guests-head');
  const headMark = el('div', 'guests-head-mark');
  headMark.append(svg('groups'));
  const headWords = el('div', 'guests-head-words');
  headWords.append(el('div', 'guests-title', 'Guest list'));
  headWords.append(el('div', 'guests-subtitle', !view.sent
    ? (view.list.length ? 'Add everyone, then send invites.' : 'Nobody on the list yet.')
    : unsent ? `${unsent} added since the invites went out, not sent yet.` : 'Invites are out.'));
  const tools = el('div', 'guests-tools');
  tools.append(button('Add people', 'plus', 'button button-small', () => openPicker(e, view, refresh)));
  if (view.sent) {
    tools.append(sendMenu(e, view, refresh));
  }
  head.append(headMark, headWords, tools);
  return head;
}

function sendMenu(e, view, refresh) {
  const waiting = view.list.filter(r => r.invited && !r.answer).length;
  const first = eventDates(e)[0];
  const day = weekdayLong(first) + ', ' + parseDate(first).toLocaleDateString('en-US', {month: 'long', day: 'numeric'});
  const when = e.allDay ? day : `${day}, ${timeLine(e)}`;
  return menuButton('Send', 'calendar', [
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

function guestStat(n, label, icon, cls, onClick) {
  const tile = el('button', 'guests-stat ' + cls);
  tile.type = 'button';
  tile.addEventListener('click', onClick);
  tile.append(svg(icon));
  const words = el('div', 'guests-stat-words');
  words.append(el('strong', '', String(n)), el('span', '', label));
  tile.append(words);
  return tile;
}

function guestStats(e, view, refresh) {
  const c = view.counts;
  const counts = el('div', 'guests-stats');
  const tableOf = answer => () => openTable(e, view, refresh, {answer});
  counts.append(
    guestStat(c.invited, 'invited', 'mail', 'is-all', tableOf('')),
    guestStat(c.yes, 'yes', 'check', 'is-yes', tableOf('yes')),
    guestStat(c.maybe, 'maybe', 'help', 'is-maybe', tableOf('maybe')),
    guestStat(c.no, 'no', 'ban', 'is-no', tableOf('no')),
    guestStat(c.waiting, 'no reply yet', 'reply', 'is-waiting', tableOf('none')),
  );
  const pending = view.list.filter(r => r.invited && !r.sent && r.email).length;
  if (pending) {
    counts.append(guestStat(pending, 'pending', 'clock', 'is-pending', () => openPending(e, view, refresh)));
  }
  if (view.sent) {
    counts.append(guestStat(view.list.filter(r => r.opened).length, 'opened', 'eye', 'is-opened', () => openTable(e, view, refresh, {opened: 'yes'})));
  }
  return counts;
}

function householdsOf(view) {
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
  return [...households.values()].sort((a, b) => (a[0].name || '').localeCompare(b[0].name || ''));
}

function guestsPartHead(title, sub) {
  const head = el('div', 'guests-part-head');
  head.append(el('div', 'guests-part-title', title), el('div', 'guests-part-sub', sub));
  return head;
}

function guestsTable(e, view, refresh) {
  const table = el('div', 'guests-table');
  const groups = view.groups || [];
  if (groups.length) {
    table.append(guestsPartHead('Groups', 'People from these groups are included.'));
  }
  for (const g of groups) {
    table.append(guestGroupBlock(e, view, g, refresh));
  }
  const households = householdsOf(view);
  if (households.length) {
    table.append(guestsPartHead('Individuals', 'Added one at a time, or came by the link.'));
  }
  for (const household of households) {
    const block = el('div', 'guests-household guests-plain-list');
    for (const r of household) {
      block.append(guestPlainRow(e, view, r, refresh));
    }
    table.append(block);
  }
  return table;
}

function guestGroupBlock(e, view, g, refresh) {
  const members = view.list.filter(r => r.via === 'group:' + g.id);
  const block = el('div', 'guests-household guests-group-block' + (openGroups.has(g.id) ? ' is-open' : ''));
  block.append(guestGroupHead(e, g, members.length, () => {
    if (openGroups.has(g.id)) {
      openGroups.delete(g.id);
    } else {
      openGroups.add(g.id);
    }
    block.classList.toggle('is-open', openGroups.has(g.id));
  }, refresh));
  const inner = el('div', 'guests-group-members');
  for (const r of members.sort((x, y) => (x.name || '').localeCompare(y.name || ''))) {
    inner.append(guestRow(e, view, r, refresh));
  }
  block.append(inner);
  return block;
}

function guestGroupHead(e, g, count, onToggle, refresh) {
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
  toggle.append(svg('chevron-down'));
  toggle.addEventListener('click', onToggle);
  row.append(toggle, guestGroupFoot(e, g, refresh));
  return row;
}

function guestGroupFoot(e, g, refresh) {
  const foot = el('div', 'guests-group-foot');
  const auto = el('label', 'guests-group-auto');
  const box = el('input');
  box.type = 'checkbox';
  box.checked = g.auto;
  box.addEventListener('change', async () => {
    try {
      await api('PUT', '/api/when/invites/group', {id: e.id, group: g.id, auto: box.checked});
      toast(box.checked ? (g.sent ? 'Auto-invite on: newcomers are sent their invitation' : 'Auto-invite on: newcomers are sent theirs once you have sent this group its invites') : 'Auto-invite off: newcomers wait in Pending for you to send');
      if (box.checked) {
        refresh();
      }
    } catch (err) {
      toast(err.message);
      box.checked = !box.checked;
    }
  });
  const info = el('span', 'guests-group-info');
  info.title = 'Whoever comes to match this group later goes on the list; with Auto-invite on they are sent the invitation too, once you have sent this group its invites.';
  info.append(svg('info'));
  auto.append(box, el('span', '', 'Auto-invite'), info);
  const remove = el('button', 'guests-action is-remove');
  remove.type = 'button';
  remove.title = 'Take the group off';
  remove.append(svg('trash'));
  remove.addEventListener('click', async () => {
    if (!confirm('Remove this group? Anyone already sent an invitation will stay, but pending guests will be removed.')) {
      return;
    }
    try {
      const made = await api('DELETE', '/api/when/invites/group', {id: e.id, group: g.id});
      toast(made.dropped ? `Group removed, and ${made.dropped} with it` : 'Group removed');
      refresh();
    } catch (err) {
      toast(err.message);
    }
  });
  foot.append(auto, remove);
  return foot;
}

function guestRow(e, view, r, refresh) {
  const row = el('div', 'guests-row' + (r.guestOf ? ' is-guest' : '') + (r.invited ? '' : ' is-link'));
  row.append(face(r));
  const who = el('div', 'invite-who');
  who.append(el('div', 'invite-name', r.name || r.email));
  const bits = [r.guestOf ? `Guest of ${r.guestOfName}` : r.line, r.email && r.outside ? r.email : ''].filter(Boolean);
  if (bits.length) {
    who.append(el('div', 'invite-line', bits.join(' \u00b7 ')));
  }
  who.append(guestMarks(e, view, r, refresh));
  row.append(who);
  const side = el('div', 'guests-side');
  side.append(answerButtons(r, e, () => refresh()));
  if (r.answer) {
    side.append(el('div', 'guests-by', answeredWords(r)));
  }
  row.append(side, guestActions(e, r, refresh));
  return row;
}

function guestMarks(e, view, r, refresh) {
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
    marks.append(warningChip(e, view, r, refresh));
  }
  return marks;
}

function guestAction(icon, title, className, onClick) {
  const action = el('button', className);
  action.type = 'button';
  action.title = title;
  action.append(svg(icon));
  action.addEventListener('click', onClick);
  return action;
}

function guestActions(e, r, refresh) {
  const actions = el('div', 'guests-actions');
  if (r.link) {
    actions.append(guestAction('link', 'Copy their own page\u2019s link - it needs no sign-in', 'guests-action', () => copyText(location.origin + r.link, 'Link copied - theirs alone, no sign-in needed')));
  }
  if (!r.guestOf && r.invited && !r.outside) {
    actions.append(guestAction('plus', 'Add a guest for ' + firstName(r), 'guests-action', () => openGuestForm(e, r.key, refresh)));
  }
  if (r.invited) {
    actions.append(guestAction('trash', 'Take off the list', 'guests-action is-remove', async () => {
      if (!confirm(`Take ${r.name || r.email} off the list? Their answer goes with them${r.via && r.via.startsWith('group:') ? ', and the group will not add them back' : ''}.`)) {
        return;
      }
      try {
        await api('DELETE', '/api/when/invites/people', {id: e.id, email: r.key});
        refresh();
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  return actions;
}

function guestPlainRow(e, view, r, refresh) {
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
}

function guestTableButton(e, view, refresh) {
  const open = el('button', 'guests-table-row');
  open.type = 'button';
  const openMark = el('div', 'guests-table-mark');
  openMark.append(svg('menu'));
  const openWords = el('div', 'guests-table-words');
  openWords.append(el('div', 'guests-table-title', 'Guest table'), el('div', 'guests-table-sub', 'View and manage your full guest list.'));
  open.append(openMark, openWords, svg('chevron-right'));
  open.addEventListener('click', () => openTable(e, view, refresh));
  return open;
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
      await api('PUT', '/api/when/invites/settings', {id: e.id, ...(invitation ? {} : {message: message.value}), ...own});
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

function permissionsForm(e, view, refresh, shut) {
  const form = el('form', 'admin-form');
  const invite = checkbox('Allow others to invite guests', view.guests, 'Anyone invited may invite more people and bring guests of their own. Off, only the hosts add to the list.');
  const list = checkbox('Allow everyone to see the guest list', !view.listPrivate, 'Everyone who can open the event sees who said yes or maybe, and who has not answered. Only the hosts see who said no.');
  const actions = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const submit = el('button', 'button');
  submit.type = 'submit';
  submit.append(svg('check'), el('span', '', 'Save'));
  actions.append(submit, status);
  form.append(invite.wrap, list.wrap, actions);
  form.addEventListener('submit', async ev => {
    ev.preventDefault();
    submit.disabled = true;
    try {
      await api('PUT', '/api/when/invites/settings', {id: e.id, guests: invite.input.checked, publicList: list.input.checked});
      toast('Saved');
      shut();
      await refresh();
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
  const own = e.source === 'sheet' && (postedAndHosting(e) || isAdmin() || host);
  const imported = (e.source === 'google' || e.source === 'pdf') && (isAdmin() || host);
  let shut = null;
  const box = el('div', 'editor');
  const panels = {};
  if (own || imported) {
    const more = host ? [{label: 'Message', panel: settingsForm(e, view, refresh, () => shut(), 'email')}, {label: 'Permissions', panel: permissionsForm(e, view, refresh, () => shut())}] : [];
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
    panels.permissions = permissionsForm(e, view, refresh, () => shut());
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
    for (const [key, label] of [['event', 'Event'], ['invitation', 'Invitation'], ['email', 'Email Invitation'], ['permissions', 'Permissions']]) {
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
      const made = await api('POST', '/api/when/invites/send', {id: e.id, to: 'sent', update: true});
      longToast(made.messages === 1 ? 'The update is on its way' : `${made.messages} updates are on their way`);
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
    card.append(button(party ? 'Invite the ticket holders' : 'Invite the volunteers', party ? 'ticket' : 'people', 'button', async () => {
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
  card.append(button('Add People', 'plus', 'button' + (isParty(e) ? ' button-secondary' : ''), () => openPicker(e, view, refresh)));
  return card;
}
