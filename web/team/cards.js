import {state, me, family, whenParts, coChairs, shownVolunteers, descendants, canJoin, isFull, mySignUp, signUpOf, activityPath, rootOf, parentOf, category, parseWhen, UNCATEGORIZED} from './state.js';
import {el, link, svg, thumb, badge, button, avatar} from './dom.js';
import {openSignUp, openActivity} from './edit.js';

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

// peopleLine is who is on a thing, one line under the row's text: the leads
// first, in ink and each marked "(Lead)", then the volunteers in sign-up
// order - one open to co-chairing marked "(Co-Chair Opt)", in the accent, so
// an offer is seen from the list - then everyone under its sub-activities
// with the sub-activity named - "Alice Che (Performance)" - up to twelve
// names in all, then "and 5 more". The server already withholds a hidden
// list from anyone who does not run the thing, so whatever arrives may be
// shown.
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

// labelLine is the small first line over a title: the day and time, each
// behind an icon, and any status badges. Who is on it is its own line below
// (peopleLine), so the two never jostle for one row.
function labelLine(node) {
  const label = el('div', 'label');
  // A thing that just happens when its parent does says nothing about when:
  // the parent's page already does, and a list of rows all saying the same
  // date is noise.
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


// completeBadge is the Volunteers Complete mark: a green check and the words,
// no ground - good news, not a warning.
export function completeBadge() {
  const mark = el('span', 'badge complete');
  mark.append(svg('check'), el('span', '', 'Volunteers Complete'));
  return mark;
}

// needChip is the co-chair-wanted chip, which sits beside the row's title.
function needChip(node) {
  if (!node.coLeaderNeeded) {
    return null;
  }
  const chip = el('span', 'need');
  chip.append(svg('people'), el('span', '', 'Co-chair needed'));
  return chip;
}

// wanted says a thing is marked a priority and still wants people: open,
// and neither complete nor with every spot taken.
export function wanted(node) {
  return Boolean(node.priority) && node.status === 'Open' && !isFull(node);
}

// priorityChip is High Priority beside a row's title, in the deep red
// behind a star, while the thing is one.
function priorityChip(node) {
  if (!wanted(node)) {
    return null;
  }
  const chip = el('span', 'need is-priority');
  chip.append(svg('star'), el('span', '', 'High Priority'));
  return chip;
}

// priorityCallout is an event card's word that it - or things under it -
// are a priority: "High priority: Decor · Check-In", each a link to the
// thing, or "High priority" alone for the event itself.
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

// childRow is the row a thing under an activity gets in its parent's list. While
// the parent is in edit mode, each row carries a pencil that opens that child's
// editor in place - fixing three titles should not mean visiting three pages.
// moves, while editing, is {up, down}: what to do when the row is nudged a
// place in either direction, null where it is already at that end.
export function childRow(node, editing, moves) {
  const row = link(activityPath(node), 'row is-link' + (node.status === 'Hidden' || node.status === 'Pending' ? ' is-muted' : ''));
  row.append(thumb(node.imageUrl || rootOf(node).imageUrl, node.title));
  const body = el('div', 'row-body');
  body.append(labelLine(node));
  const title = el('div', 'row-title', node.title);
  // The asks sit right by the name, where they are read first: High
  // Priority, then Co-chair needed.
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
    // What sits under it, each a small chip - plainly not part of the
    // description - naming the committee with how many are on it, chairs
    // included, as "3" or, against a cap, "2 of 5". The row is itself a
    // link, so a chip is a button that goes to the committee instead (an
    // anchor inside an anchor is not a thing).
    const under = node.children.filter(c => c.status !== 'Hidden' && c.status !== 'Pending');
    if (under.length) {
      const line = el('div', 'row-under');
      for (const c of under) {
        const chip = button(`${c.title} (${c.spots > 0 ? `${c.taken} of ${c.spots}` : c.taken})`, null, 'row-under-chip', async () => {
          const {navigate} = await import('./app.js');
          navigate(activityPath(c));
        });
        chip.title = `Open ${c.title}`;
        line.append(chip);
      }
      body.append(line);
    }
  }
  // Who is on it comes last, leads first.
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
    // While editing, a row can be picked up and dropped on another category's
    // panel (detail.js wires the targets); the row carries its id along.
    row.draggable = true;
    row.addEventListener('dragstart', e => {
      e.dataTransfer.setData('text/plain', node.id);
      e.dataTransfer.effectAllowed = 'move';
      row.classList.add('is-dragging');
    });
    row.addEventListener('dragend', () => row.classList.remove('is-dragging'));
    // A place up or down among its siblings, for anyone not dragging.
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
    // button() swallows the click so the row's link does not fire underneath.
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

// dateBadge is the little month-over-day stamp in a card's image corner. An
// activity with no parsed date (a committee that runs all year, an idea nobody
// has scheduled) gets its timing word instead, so the corner is never empty.
function dateBadge(act) {
  const start = parseWhen(act.start);
  if (act.timing || !start) {
    return act.timing ? el('div', 'card-stamp card-stamp-text', act.timing) : null;
  }
  const stamp = el('div', 'card-stamp');
  const end = parseWhen(act.end);
  const spans = end && end.date.toDateString() !== start.date.toDateString();
  const sameMonth = spans && end.date.getMonth() === start.date.getMonth() && end.date.getFullYear() === start.date.getFullYear();
  // A tear-off calendar: the month as the red band across the top, the day
  // large, and the weekday under it - for a single day only; a span of days
  // reads "3-5" under its month, or "30-1" under "OCT-NOV" when it crosses
  // into the next.
  const month = monthFormat.format(start.date).toUpperCase();
  stamp.append(el('div', 'card-stamp-month', spans && !sameMonth ? `${month}-${monthFormat.format(end.date).toUpperCase()}` : month),
    el('div', 'card-stamp-day', spans ? `${start.date.getDate()}-${end.date.getDate()}` : String(start.date.getDate())));
  if (!spans) {
    stamp.append(el('div', 'card-stamp-dow', start.date.toLocaleDateString('en-US', {weekday: 'short'}).toUpperCase()));
  }
  return stamp;
}

// The card's corner counts what is left; the Volunteers Complete mark above
// already says when nothing is.
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

// activityCard is the grid tile the opportunities page shows: image with its
// date stamp - and a chip when a co-chair is wanted, the one thing worth
// flagging on the picture; the category is the heading the card sits under -
// then title, blurb, and the sign-up action.
// opts.signUps lists one person's sign-ups on the activity and under it, for
// My Sign Ups - opts.email says whose: the viewer's, or someone in their
// household - each a link with a check - the event itself by its own title
// when signed up for directly - with its own date when it has one, and a
// pencil into the sign-up itself, where it can be changed or removed; all in
// place of the foot, since the card says where they are, not what to join.
export function activityCard(act, opts = {}) {
  const slot = el('div', 'card-slot ' + categoryClass(act.category));
  slot.append(activityCardBody(act, opts));
  return slot;
}

// activityCardBody is the card itself; activityCard puts it in a slot with a
// card of its category's colour peeking out behind, as Heliosian's do.
function activityCardBody(act, opts) {
  const card = el('div', 'card' + (act.status === 'Hidden' || act.status === 'Pending' ? ' is-muted' : ''));
  const media = link(activityPath(act), 'card-media');
  // Without a photo the tile falls back to a big initial; tinting it by category
  // keeps a grid of image-less activities from reading as a wall of one colour.
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
      // Chairing it, or offering to, is tagged the way the faces on its page
      // tag it.
      const mine = signUpOf(node, opts.email || me().email);
      if (mine && mine.position === 'Co-Chair') {
        item.append(el('span', 'side-chair-role', 'Chair'));
      } else if (mine && mine.position === 'Open to Co-Chair') {
        item.append(el('span', 'side-chair-role is-option', 'Chair opt'));
      }
      // The card's stamp already dates the event itself.
      const when = node.whenFrom || node === act ? {} : whenParts(node);
      const own = [when.words, when.day, when.time].filter(Boolean).join(' · ');
      if (own) {
        item.append(el('span', 'card-under-when', own));
      }
      if (mine) {
        // button() swallows the click so the item's link does not fire.
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

// priorityRow is one row of the High Priority panel: its picture - its
// own, else its event's - the things above it over its title and a line
// of what it is and its category as a small tag, its day and hours, the faces of who
// is on it - its chair ringed in yellow - or Co-chair needed, and Sign up; the
// whole row opens its page.
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
    main.append(el('span', 'prio-tag', heading.title));
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
  const faces = el('div', 'prio-faces');
  // Chairs first, ringed in yellow, then those open to co-chairing, in
  // teal, as the page's own faces are.
  const rank = v => v.position === 'Co-Chair' ? 0 : v.position === 'Open to Co-Chair' ? 1 : 2;
  for (const v of [...shownVolunteers(node)].sort((a, b) => rank(a) - rank(b)).slice(0, 3)) {
    faces.append(avatar(v, 'prio-face' + (rank(v) === 0 ? ' is-chair' : rank(v) === 1 ? ' is-option' : '')));
  }
  if (faces.children.length) {
    people.append(faces);
  }
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

// rolesLine is the household's part in an event, as Heliosian's calendar
// tells it: the viewer's own roles named - "Co-Chair · Performance Tech" -
// then, on a row of its own with the family's mark, the rest of the
// household as a count of roles. Nothing when none of them is on it.
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
    // One role a line: bulleted together they wrap mid-name in a card.
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

// Categories come from the sheet, so the tint is picked by position rather than
// by name - a renamed or newly added category still gets a colour. The order is
// read from the model on every call rather than pushed in by whichever page
// happens to render first, so a link straight to an activity tints it the same
// way the grid does.
export function categoryClass(id) {
  if (id === UNCATEGORIZED) {
    return 'cat-none';
  }
  const i = (state.model ? state.model.categories : []).findIndex(c => c.id === id);
  return 'cat-' + (i < 0 ? 1 : i % 5 + 1);
}
