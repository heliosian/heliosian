import {el, svg, button, longToast, copyText} from '/elements.js';
import {popup} from '/modal.js';
import {act} from '/data.js';
import {chipToggle, filterControl} from '/rules.js';
import {answerWords, answerIcon, answerButtons, face, ticketWords, stamp} from './inviteparts.js';

export function listFilters(all, view, onChange, {answer = '', opened = '', rsvp = true} = {}) {
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

export function openTable(e, view, refresh, {answer = '', opened = ''} = {}) {
  const box = el('div');
  const rows = el('div', 'guest-table-wrap');
  const count = el('div', 'picker-note');
  let shownNow = view.list;
  const shown = () => shownNow;
  const columns = ['Name', 'Email', 'Grade', 'RSVP', ...(view.party ? ['Ticket'] : []), ...(view.sent ? ['Opened'] : [])];
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
        tr.append(el('td', 'guest-table-opened', r.opened ? stamp(r.opened) : r.sent ? '—' : ''));
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

export function openMessage(e, view, refresh, preset = {}) {
  const box = el('div', 'compose');
  const head = el('div', 'compose-head');
  const mark = el('div', 'compose-mark');
  mark.append(svg('chat'));
  const words = el('div');
  words.append(el('h2', 'compose-title', preset.title || 'Send a message'), el('p', 'compose-lead', 'Invites and messages to students are sent to their parents too.'));
  head.append(mark, words);
  box.append(head);
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
  box.append(el('div', 'compose-label', 'Subject'));
  const subject = el('input');
  subject.type = 'text';
  subject.required = true;
  subject.maxLength = 200;
  subject.placeholder = 'What to bring, a change of plan, a reminder…';
  subject.value = preset.subject || '';
  const subjectRow = el('div', 'message-subject');
  subjectRow.append(el('span', 'message-subject-prefix', `[${e.title}]`), subject);
  box.append(subjectRow);
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
      await act('events', e.id, 'message', {subject: subject.value.trim(), message: message.value.trim(), to: [...to], emails: [...picked], attach: Boolean(preset.attach)});
      const reached = view.list.filter(r => r.invited && r.email && (to.has(r.answer && r.answer !== 'hidden' ? r.answer : 'none') || picked.has(r.key))).length;
      longToast(reached === 1 ? 'Sent to one person' : `Sent to ${reached} people`);
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

function openPick(e, view, picked, onDone) {
  const box = el('div');
  const search = el('input', 'picker-search');
  search.type = 'search';
  search.placeholder = 'Search the guest list';
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
      const said = el('span', 'invite-said pick-said is-' + (r.answer || 'none'));
      const mark = el('span', 'invite-said-mark');
      mark.append(svg(answerIcon(r.answer)));
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
    done.replaceChildren(svg('check'), el('span', '', picked.size ? `Done · ${picked.size} picked` : 'Done'));
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
