import {whenLabel, coChairs, canJoin, isFull, mySignUp, activityPath, rolePath} from './state.js';
import {el, link, svg, thumb, badge, button} from './dom.js';
import {openSignUp} from './edit.js';

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
function labelLine(node, withChairs) {
  const label = el('div', 'label');
  const when = whenLabel(node);
  if (when) {
    label.append(el('span', '', when));
  }
  if (withChairs) {
    const chairs = coChairs(node).map(v => v.name).join(', ');
    if (chairs) {
      label.append(el('span', '', chairs));
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

function joinButton(act, node) {
  if (mySignUp(node)) {
    const done = button('Signed up', 'join', 'button button-secondary button-small');
    done.disabled = true;
    return done;
  }
  if (!canJoin(act, node)) {
    return null;
  }
  return button('Join', 'join', 'button button-small', () => openSignUp(act, node === act ? null : node, null));
}

// activityRow is the card the sign-up and my-activities lists share.
export function activityRow(act, options) {
  const opts = options || {};
  const node = opts.role || act;
  const row = link(opts.href || activityPath(act), 'row is-link' + (act.status === 'Hidden' || act.status === 'Pending' ? ' is-muted' : ''));
  row.append(thumb(node.imageUrl || act.imageUrl, act.title));
  const body = el('div', 'row-body');
  body.append(labelLine(node, false));
  const title = el('div', 'row-title', act.title);
  if (opts.role) {
    title.append(el('span', 'chain', '▶'), el('span', '', opts.role.title));
  }
  body.append(title);
  const text = opts.role ? opts.role.description : act.description;
  if (text) {
    body.append(el('div', 'row-text clamp', text));
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  if (!opts.noJoin) {
    const join = joinButton(act, act);
    if (join) {
      actions.append(join);
    }
  }
  if (opts.actions) {
    for (const a of opts.actions) {
      actions.append(a);
    }
  }
  actions.append(svg('chevron'));
  actions.querySelector('svg:last-child').classList.add('chevron');
  row.append(actions);
  return row;
}

export function roleRow(act, role) {
  const row = link(rolePath(act, role), 'row is-link' + (role.status === 'Hidden' || role.status === 'Pending' ? ' is-muted' : ''));
  row.append(thumb(role.imageUrl || act.imageUrl, role.title));
  const body = el('div', 'row-body');
  body.append(labelLine(role, true));
  body.append(el('div', 'row-title', role.title));
  if (role.description) {
    body.append(el('div', 'row-text clamp', role.description));
  }
  if (role.roles.length) {
    body.append(el('div', 'row-text', `${role.roles.length} more under this`));
  }
  row.append(body);
  const actions = el('div', 'row-actions');
  const join = joinButton(act, role);
  if (join) {
    actions.append(join);
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
