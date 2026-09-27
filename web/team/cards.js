import {state, me, family, whenParts, coChairs, shownVolunteers, descendants, canJoin, isFull, mySignUp, signUpOf, activityPath, rootOf, parentOf, category, UNCATEGORIZED} from './state.js';
import {parseWhen} from '/datecard.js';
import {el, link, svg, thumb, badge, button} from './dom.js';
import {openSignUp, openActivity} from './edit.js';
import {navigate} from '/router.js';

function statusBadges(node) {
  const out = [];
  if (node.status === 'Pending') {
    out.push(badge('Needs approval', 'pending'));
  }
  if (node.status === 'Hidden') {
    out.push(badge('Hidden', 'hidden'));
  }
  if (node.status === 'Done') {
    out.push(badge('Done', 'done'));
  }
  if (node.status === 'Open' && isFull(node)) {
    out.push(completeBadge());
  }
  return out;
}

function peopleLine(node, editing) {
  const line = el('div', 'row-people');
  const parts = [];
  for (const v of coChairs(node)) {
    const lead = el('span', 'row-lead', `${v.name} (Lead)`);
    parts.push(lead);
  }
  const names = shownVolunteers(node, editing).filter(v => v.position !== 'Co-Chair')
    .map(v => (v.position === 'Open to Co-Chair' ? el('span', 'row-option', `${v.name} (Co-Chair Opt)`) : v.name));
  for (const sub of descendants(node)) {
    for (const v of shownVolunteers(sub, editing)) {
      names.push(`${v.name} (${sub.title})`);
    }
  }
  const room = Math.max(0, 12 - parts.length);
  for (const name of names.slice(0, room)) {
    parts.push(typeof name === 'string' ? el('span', '', name) : name);
  }
  if (!parts.length) {
    return null;
  }
  parts.forEach((part, i) => {
    if (i > 0) {
      line.append(', ');
    }
    line.append(part);
  });
  if (names.length > room) {
    line.append(`, and ${names.length - room} more`);
  }
  return line;
}

function labelLine(node) {
  const label = el('div', 'label');
  const when = node.whenFrom ? {} : whenParts(node);
  const iconed = (icon, text) => {
    const span = el('span', 'label-when');
    span.append(svg(icon), el('span', '', text));
    return span;
  };
  if (when.words) {
    label.append(iconed('calendar', when.words));
  }
  if (when.day) {
    label.append(iconed('calendar', when.day));
  }
  if (when.time) {
    label.append(iconed('clock', when.time));
  }
  for (const b of statusBadges(node)) {
    label.append(b);
  }
  return label;
}


export function completeBadge() {
  const mark = el('span', 'badge complete');
  mark.append(svg('check'), el('span', '', 'Volunteers Complete'));
  return mark;
}

function needChip(node) {
  if (!node.coLeaderNeeded) {
    return null;
  }
  const chip = el('span', 'need');
  chip.append(svg('people'), el('span', '', 'Co-chair needed'));
  return chip;
}

export function wanted(node) {
  return Boolean(node.priority) && node.status === 'Open' && !isFull(node);
}

function priorityChip(node) {
  if (!wanted(node)) {
    return null;
  }
  const chip = el('span', 'need is-priority');
  chip.append(svg('star'), el('span', '', 'High Priority'));
  return chip;
}

function priorityCallout(act) {
  const inside = descendants(act).filter(wanted);
  if (!wanted(act) && !inside.length) {
    return null;
  }
  const line = el('div', 'card-priority-callout');
  line.append(svg('star'));
  const words = el('span', 'card-priority-words');
  words.append(el('strong', '', inside.length ? 'High priority: ' : 'High priority'));
  inside.forEach((n, i) => {
    if (i) {
      words.append(document.createTextNode(' \u00b7 '));
    }
    words.append(link(activityPath(n), 'card-priority-link', n.title));
  });
  line.append(words);
  return line;
}

function joinButton(node) {
  if (mySignUp(node)) {
    const done = button('Signed up', 'join', 'button button-secondary button-small');
    done.disabled = true;
    return done;
  }
  if (!canJoin(node)) {
    return null;
  }
  return button('Join', 'join', 'button button-small', () => openSignUp(node, null));
}

export function childRow(node, editing, moves) {
  const row = link(activityPath(node), 'row is-link' + (node.status === 'Hidden' || node.status === 'Pending' ? ' is-muted' : ''));
  row.append(thumb(node.imageUrl || rootOf(node).imageUrl, node.title));
  const body = el('div', 'row-body');
  body.append(labelLine(node));
  const title = el('div', 'row-title', node.title);
  const urgent = priorityChip(node);
  if (urgent) {
    title.append(urgent);
  }
  const need = needChip(node);
  if (need) {
    title.append(need);
  }
  body.append(title);
  if (node.description) {
    body.append(el('div', 'row-text clamp', node.description));
  }
  if (node.children.length) {
    const under = node.children.filter(c => c.status !== 'Hidden' && c.status !== 'Pending');
    if (under.length) {
      const line = el('div', 'row-under');
      for (const c of under) {
        const chip = button(`${c.title} (${c.spots > 0 ? `${c.taken} of ${c.spots}` : c.taken})`, null, 'row-under-chip', () => navigate(activityPath(c)));
        chip.title = `Open ${c.title}`;
        line.append(chip);
      }
      body.append(line);
    }
  }
  const people = peopleLine(node, editing);
  if (people) {
    body.append(people);
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  const join = joinButton(node);
  if (join) {
    actions.append(join);
  }
  if (editing) {
    row.draggable = true;
    row.addEventListener('dragstart', e => {
      e.dataTransfer.setData('text/plain', node.id);
      e.dataTransfer.effectAllowed = 'move';
      row.classList.add('is-dragging');
    });
    row.addEventListener('dragend', () => row.classList.remove('is-dragging'));
    if (moves) {
      const nudge = el('div', 'row-nudge');
      for (const [dir, fn] of [['up', moves.up], ['down', moves.down]]) {
        const b = button('', dir, 'edit-icon', fn || (() => {}));
        b.title = `Move ${dir}`;
        b.setAttribute('aria-label', `Move ${node.title} ${dir}`);
        b.disabled = !fn;
        nudge.append(b);
      }
      actions.append(nudge);
    }
    const pencil = button('', 'edit', 'edit-icon', () => openActivity(node));
    pencil.title = `Edit ${node.title}`;
    pencil.setAttribute('aria-label', pencil.title);
    actions.append(pencil);
  }
  const chevron = svg('chevron');
  chevron.classList.add('chevron');
  actions.append(chevron);
  row.append(actions);
  return row;
}

export function linkRow(act, item, editor, onEdit) {
  const row = el('div', 'row');
  row.append(thumb(item.imageUrl || act.imageUrl, item.title, 'small'));
  const body = el('div', 'row-body');
  body.append(el('div', 'row-title', item.title));
  const anchor = el('a', 'row-text', item.url.replace(/^https?:\/\//, ''));
  anchor.href = item.url;
  anchor.target = '_blank';
  anchor.rel = 'noopener';
  body.append(anchor);
  row.append(body);
  const actions = el('div', 'row-actions');
  const open = button('Open', 'open', 'button button-small', () => window.open(item.url, '_blank', 'noopener'));
  actions.append(open);
  if (editor) {
    actions.append(button('Edit', 'edit', 'button button-small', onEdit));
  }
  row.append(actions);
  return row;
}

const monthFormat = new Intl.DateTimeFormat('en-US', {month: 'short'});

function dateBadge(act) {
  const start = parseWhen(act.start);
  if (act.timing || !start) {
    return act.timing ? el('div', 'card-stamp card-stamp-text', act.timing) : null;
  }
  const stamp = el('div', 'card-stamp');
  const end = parseWhen(act.end);
  const spans = end && end.date.toDateString() !== start.date.toDateString();
  const sameMonth = spans && end.date.getMonth() === start.date.getMonth() && end.date.getFullYear() === start.date.getFullYear();
  const month = monthFormat.format(start.date).toUpperCase();
  stamp.append(el('div', 'card-stamp-month', spans && !sameMonth ? `${month}-${monthFormat.format(end.date).toUpperCase()}` : month),
    el('div', 'card-stamp-day', spans ? `${start.date.getDate()}-${end.date.getDate()}` : String(start.date.getDate())));
  if (!spans) {
    stamp.append(el('div', 'card-stamp-dow', start.date.toLocaleDateString('en-US', {weekday: 'short'}).toUpperCase()));
  }
  return stamp;
}

function spotsNote(act) {
  if (isFull(act)) {
    return '';
  }
  if (act.spots > 0) {
    const left = act.spots - act.taken;
    return `${left} ${left === 1 ? 'spot' : 'spots'} left`;
  }
  return '';
}

export function activityCard(act, opts = {}) {
  const slot = el('div', 'card-slot ' + categoryClass(act.category));
  slot.append(activityCardBody(act, opts));
  return slot;
}

function activityCardBody(act, opts) {
  const card = el('div', 'card' + (act.status === 'Hidden' || act.status === 'Pending' ? ' is-muted' : ''));
  const media = link(activityPath(act), 'card-media');
  media.append(thumb(act.imageUrl, act.title, 'card-image ' + categoryClass(act.category)));
  if (act.coLeaderNeeded) {
    const need = el('span', 'card-chip card-need');
    need.append(svg('people'), el('span', '', 'Co-chair needed'));
    media.append(need);
  }
  const stamp = dateBadge(act);
  if (stamp) {
    media.append(stamp);
  }
  card.append(media);
  const body = el('div', 'card-body');
  const title = link(activityPath(act), 'card-title');
  title.textContent = act.title;
  body.append(title);
  if (act.description) {
    body.append(el('div', 'card-text clamp', act.description));
  }
  if (!opts.signUps) {
    const callout = priorityCallout(act);
    if (callout) {
      body.append(callout);
    }
  }
  const marks = el('div', 'card-marks');
  for (const b of statusBadges(act)) {
    marks.append(b);
  }
  if (marks.children.length) {
    body.append(marks);
  }
  if (opts.signUps) {
    const under = el('div', 'card-under');
    for (const node of opts.signUps) {
      const item = link(activityPath(node), 'card-under-item');
      item.append(svg('join'), el('span', 'card-under-title', node.title));
      const mine = signUpOf(node, opts.email || me().email);
      if (mine && mine.position === 'Co-Chair') {
        item.append(el('span', 'side-chair-role', 'Chair'));
      } else if (mine && mine.position === 'Open to Co-Chair') {
        item.append(el('span', 'side-chair-role is-option', 'Chair opt'));
      }
      const when = node.whenFrom || node === act ? {} : whenParts(node);
      const own = [when.words, when.day, when.time].filter(Boolean).join(' · ');
      if (own) {
        item.append(el('span', 'card-under-when', own));
      }
      if (mine) {
        const pencil = button('', 'edit', 'edit-icon card-under-edit', () => openSignUp(node, mine));
        pencil.title = 'Edit or remove this sign-up';
        pencil.setAttribute('aria-label', `Edit ${mine.name}'s sign-up for ${node.title}`);
        item.append(pencil);
      }
      under.append(item);
    }
    body.append(under);
  }
  if (!opts.signUps) {
    const roles = rolesLine(act);
    if (roles) {
      body.append(roles);
    }
  }
  card.append(body);
  if (opts.signUps) {
    return card;
  }
  const foot = el('div', 'card-foot');
  const join = joinButton(act);
  foot.append(join || link(activityPath(act), 'button button-secondary button-small', 'Learn More'));
  const note = spotsNote(act);
  if (note) {
    foot.append(el('span', 'card-note', note));
  }
  card.append(foot);
  return card;
}

export function priorityRow(node) {
  const root = rootOf(node);
  const row = link(activityPath(node), 'prio-row');
  row.append(thumb(node.imageUrl || root.imageUrl, node.title, 'prio-pic'));
  const main = el('div', 'prio-main');
  const above = [];
  for (let up = parentOf(node); up; up = parentOf(up)) {
    above.unshift(up.title);
  }
  if (above.length) {
    main.append(el('div', 'prio-eyebrow', above.join(' \u203a ')));
  }
  main.append(el('div', 'prio-title', node.title));
  if (node.description) {
    main.append(el('div', 'prio-text clamp', node.description));
  }
  const heading = category(root.category);
  if (heading) {
    main.append(el('span', 'prio-tag ' + categoryClass(root.category), heading.title));
  }
  row.append(main);
  const when = el('div', 'prio-when');
  const start = parseWhen(node.start);
  if (node.timing) {
    const line = el('div', 'prio-when-line');
    line.append(svg('calendar'), el('span', '', node.timing));
    when.append(line);
  } else if (start) {
    const end = parseWhen(node.end);
    const day = el('div', 'prio-when-line');
    const dayWords = start.date.toLocaleDateString('en-US', {weekday: 'short', month: 'short', day: 'numeric', year: 'numeric'});
    const spans = end && end.date.toDateString() !== start.date.toDateString();
    day.append(svg('calendar'), el('span', '', spans ? `${dayWords} \u2013 ${end.date.toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}` : dayWords));
    when.append(day);
    if (start.hasTime && !spans) {
      const time = el('div', 'prio-when-line');
      const at = d => d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
      time.append(svg('clock'), el('span', '', end && end.hasTime ? `${at(start.date)} \u2013 ${at(end.date)}` : at(start.date)));
      when.append(time);
    }
  }
  row.append(when);
  const people = el('div', 'prio-people');
  if (!coChairs(node).length && node.coLeaderNeeded) {
    people.append(el('span', 'prio-chair is-needed', 'Co-chair needed'));
  }
  row.append(people);
  const act = el('div', 'prio-action');
  if (mySignUp(node)) {
    const done = button('Signed up', 'join', 'button button-secondary button-small');
    done.disabled = true;
    act.append(done);
  } else if (canJoin(node)) {
    act.append(button('Sign up', null, 'button prio-signup', () => openSignUp(node, null)));
  }
  row.append(act);
  const chevron = svg('chevron');
  chevron.classList.add('chevron');
  row.append(chevron);
  return row;
}

function rolesLine(act) {
  const mine = me().email;
  const household = new Set(family().map(c => c.email));
  const own = [];
  let others = 0;
  for (const node of [act, ...descendants(act)]) {
    if (node.status === 'Hidden' || node.status === 'Pending') {
      continue;
    }
    for (const v of node.volunteers) {
      if (v.email === mine) {
        own.push(v.position === 'Co-Chair' ? `Co-Chair · ${node.title}` : node.title);
      } else if (household.has(v.email)) {
        others++;
      }
    }
  }
  if (!own.length && !others) {
    return null;
  }
  const line = el('div', 'card-roles');
  const row = (icon, className, words) => {
    const r = el('span', 'card-roles-row');
    r.append(svg(icon), el('span', className, words));
    line.append(r);
  };
  if (own.length) {
    const list = el('span', 'card-role');
    own.forEach(words => list.append(el('span', 'card-role-item', words)));
    const r = el('span', 'card-roles-row');
    r.append(svg('people'), list);
    line.append(r);
  }
  if (others) {
    row('family', 'card-role-family', `${others} role${others === 1 ? '' : 's'} in my family`);
  }
  return line;
}

export function categoryClass(id) {
  if (id === UNCATEGORIZED) {
    return 'cat-none';
  }
  const i = (state.model ? state.model.categories : []).findIndex(c => c.id === id);
  return 'cat-' + (i < 0 ? 1 : i % 5 + 1);
}
