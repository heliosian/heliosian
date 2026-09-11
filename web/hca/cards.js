import {state, whenLabel, coChairs, canJoin, isFull, mySignUp, activityPath, rootOf, category, parseWhen, UNCATEGORIZED} from './state.js';
import {el, link, svg, thumb, badge, button} from './dom.js';
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
    out.push(badge('Full', 'full'));
  }
  return out;
}

// labelLine is the small-caps line: when it happens, who chairs a role, and
// the co-leader call.
// peopleLine is who is on a thing, co-chairs first and starred, then everyone
// else in sign-up order. The server already withholds a hidden list from
// anyone who does not run the thing, so whatever arrives may be shown.
function peopleLine(node) {
  const chairs = coChairs(node).map(v => v.name + '*');
  const others = node.volunteers.filter(v => v.position !== 'Co-Chair').map(v => v.name);
  return [...chairs, ...others].join(', ');
}

function labelLine(node, withPeople) {
  const label = el('div', 'label');
  const when = whenLabel(node);
  if (when) {
    label.append(el('span', '', when));
  }
  if (withPeople) {
    const people = peopleLine(node);
    if (people) {
      label.append(el('span', 'label-people', people));
    }
  }
  if (node.coLeaderNeeded) {
    label.append(el('span', 'need', '• Co-leader needed!'));
  }
  for (const b of statusBadges(node)) {
    label.append(b);
  }
  return label;
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
export function childRow(node, editing) {
  const row = link(activityPath(node), 'row is-link' + (node.status === 'Hidden' || node.status === 'Pending' ? ' is-muted' : ''));
  row.append(thumb(node.imageUrl || rootOf(node).imageUrl, node.title));
  const body = el('div', 'row-body');
  body.append(labelLine(node, true));
  body.append(el('div', 'row-title', node.title));
  if (node.description) {
    body.append(el('div', 'row-text clamp', node.description));
  }
  if (node.children.length) {
    body.append(el('div', 'row-text', `${node.children.length} more under this`));
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
  if (!start) {
    return act.timing ? el('div', 'card-stamp card-stamp-text', act.timing) : null;
  }
  const stamp = el('div', 'card-stamp');
  const end = parseWhen(act.end);
  const sameMonth = end && end.date.getMonth() === start.date.getMonth();
  const days = end && end.date.getDate() !== start.date.getDate() && sameMonth
    ? `${start.date.getDate()}-${end.date.getDate()}`
    : String(start.date.getDate());
  stamp.append(el('div', 'card-stamp-month', monthFormat.format(start.date).toUpperCase()),
    el('div', 'card-stamp-day', days));
  return stamp;
}

function spotsNote(act) {
  if (isFull(act)) {
    return 'Full';
  }
  if (act.spots > 0) {
    const left = act.spots - act.taken;
    return `${left} ${left === 1 ? 'spot' : 'spots'} left`;
  }
  return act.coLeaderNeeded ? 'Co-leader needed' : '';
}

// activityCard is the grid tile the opportunities page shows: image with its
// category chip and date stamp, then title, blurb, and the sign-up action.
export function activityCard(act) {
  const card = el('div', 'card' + (act.status === 'Hidden' || act.status === 'Pending' ? ' is-muted' : ''));
  const media = link(activityPath(act), 'card-media');
  // Without a photo the tile falls back to a big initial; tinting it by category
  // keeps a grid of image-less activities from reading as a wall of one colour.
  media.append(thumb(act.imageUrl, act.title, 'card-image ' + categoryClass(act.category)));
  const cat = category(act.category);
  if (cat) {
    media.append(el('span', 'card-chip ' + categoryClass(act.category), cat.title));
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
  const marks = el('div', 'card-marks');
  for (const b of statusBadges(act)) {
    marks.append(b);
  }
  if (marks.children.length) {
    body.append(marks);
  }
  card.append(body);
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
