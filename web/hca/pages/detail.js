import {me, isAdmin, years, allRoles, parentOf, whenLabel, longDate, parseWhen, coChairs, mySignUp, canJoin, isFull, matches, activityPath, rolePath} from '../state.js';
import {el, link, svg, thumb, avatar, badge, button, searchBox, tabs, copyText} from '../dom.js';
import {setTitle} from '../chrome.js';
import {roleRow, linkRow} from '../cards.js';
import {openSignUp, openActivity, openRole, openLink, removeVolunteer, copyToNextYear, saveActivityFields, saveRoleFields} from '../edit.js';

function crumb(parts) {
  const nav = el('div', 'crumb');
  const back = link(parts[parts.length - 2].href, '');
  back.append(svg('back'));
  nav.append(back);
  parts.forEach((part, i) => {
    if (i) {
      nav.append(el('span', '', '/'));
    }
    if (part.href && i < parts.length - 1) {
      nav.append(link(part.href, '', part.label));
    } else {
      nav.append(el('span', 'current', part.label));
    }
  });
  return nav;
}

function header(node, act) {
  const head = el('div', 'detail-head');
  head.append(thumb(node.imageUrl || act.imageUrl, node.title));
  const body = el('div', 'row-body');
  const label = el('div', 'label');
  const when = whenLabel(node);
  if (when) {
    label.append(el('span', '', when));
  }
  if (node.coLeaderNeeded) {
    label.append(el('span', 'need', '• Co-leader needed!'));
  }
  if (node.status !== 'Open') {
    label.append(badge(node.status === 'Pending' ? 'Needs approval' : node.status, node.status.toLowerCase()));
  }
  if (node.status === 'Open' && isFull(node)) {
    label.append(badge('Full', 'full'));
  }
  body.append(label, el('h1', '', node.title));
  if (node.description) {
    body.append(el('div', 'row-text', node.description));
  }
  const meta = [];
  if (node.location) {
    meta.push(node.location);
  }
  if (node.spots) {
    meta.push(`${node.taken} of ${node.spots} spots taken`);
  }
  if (meta.length) {
    body.append(el('div', 'detail-meta', meta.join(' · ')));
  }
  head.append(body);
  return head;
}

function pad(n) {
  return String(n).padStart(2, '0');
}

function icsStamp(date, hasTime) {
  const day = `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}`;
  return hasTime ? `${day}T${pad(date.getHours())}${pad(date.getMinutes())}00` : day;
}

// downloadCalendar hands the browser a one-event calendar file with floating
// local times, the same wall-clock the sheet holds.
function downloadCalendar(node, act) {
  const start = parseWhen(node.start);
  const end = parseWhen(node.end);
  const lines = ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//HCA//Volunteer Portal//EN', 'BEGIN:VEVENT',
    `UID:${encodeURIComponent(act.year + act.title + node.title)}@hca.heliosian.com`,
    `DTSTAMP:${icsStamp(new Date(), true)}Z`];
  if (start.hasTime) {
    lines.push(`DTSTART:${icsStamp(start.date, true)}`);
    if (end) {
      lines.push(`DTEND:${icsStamp(end.date, true)}`);
    }
  } else {
    lines.push(`DTSTART;VALUE=DATE:${icsStamp(start.date, false)}`);
    const last = new Date((end || start).date);
    last.setDate(last.getDate() + 1);
    lines.push(`DTEND;VALUE=DATE:${icsStamp(last, false)}`);
  }
  const escape = s => s.replace(/\\/g, '\\\\').replace(/\n/g, '\\n').replace(/,/g, '\\,').replace(/;/g, '\\;');
  lines.push(`SUMMARY:${escape(node === act ? act.title : `${act.title}: ${node.title}`)}`);
  if (node.location || act.location) {
    lines.push(`LOCATION:${escape(node.location || act.location)}`);
  }
  if (node.description) {
    lines.push(`DESCRIPTION:${escape(node.description)}`);
  }
  lines.push(`URL:${location.href}`, 'END:VEVENT', 'END:VCALENDAR');
  const blob = new Blob([lines.join('\r\n')], {type: 'text/calendar'});
  const a = el('a');
  a.href = URL.createObjectURL(blob);
  a.download = `${node.title}.ics`;
  a.click();
  URL.revokeObjectURL(a.href);
}

function linksBand(act, role, node) {
  const editor = act.canEdit;
  if (!node.links.length && !editor) {
    return null;
  }
  const band = el('div', 'links-band');
  for (const item of node.links) {
    band.append(linkRow(act, item, editor, () => openLink(act, role, item)));
  }
  if (editor) {
    const row = el('div', 'row');
    row.append(button('Add Link', 'plus', 'button button-small', () => openLink(act, role, null)));
    band.append(row);
  }
  return band;
}

function signUpBand(act, role, node) {
  const mine = mySignUp(node);
  const band = el('div', 'signup-band');
  if (mine) {
    band.append(el('div', 'status', `You're signed up as ${mine.position.toLowerCase()}${mine.note ? ` (${mine.note})` : ''}`));
    const actions = el('div', 'center-actions');
    actions.append(button('Edit my sign-up', 'edit', 'button', () => openSignUp(act, role, mine)));
    actions.append(button('Remove me', null, 'button button-secondary', () => removeVolunteer(act, role, mine)));
    if (act.canEdit) {
      actions.append(button('Sign up someone else', 'plus', 'button button-secondary', () => openSignUp(act, role, null)));
    }
    band.append(actions);
    return band;
  }
  if (canJoin(act, node) || act.canEdit) {
    band.append(el('div', 'label', 'Sign up below (or sign up someone else)'));
    band.append(button('Sign Up', 'join', 'button', () => openSignUp(act, role, null)));
    return band;
  }
  if (node.status === 'Open' && isFull(node)) {
    band.append(el('div', 'status', 'Every spot is taken. Thank you, everyone!'));
    return band;
  }
  return null;
}

function chairsSection(node) {
  const chairs = coChairs(node);
  if (!chairs.length) {
    return null;
  }
  const wrap = el('div');
  wrap.append(el('h2', 'section', 'Co-Chairs'));
  const list = el('div', 'chairs');
  for (const v of chairs) {
    const card = el('div', 'chair');
    card.append(avatar(v), el('div', '', v.name + '*'));
    if (v.note) {
      card.append(el('div', 'row-text', v.note));
    }
    list.append(card);
  }
  wrap.append(list);
  return wrap;
}

function personCard(act, role, v) {
  const card = el('div', 'person-card');
  if (v.photoUrl) {
    const img = el('img', 'photo');
    img.src = v.photoUrl;
    img.alt = '';
    img.loading = 'lazy';
    card.append(img);
  } else {
    card.append(el('div', 'photo initial', v.name.slice(0, 1).toUpperCase()));
  }
  card.append(el('div', 'name', v.name + (v.position === 'Co-Chair' ? '*' : '')));
  const sub = [role ? role.title : act.title];
  if (v.position === 'Open to Co-Chair') {
    sub.push('Open to co-chairing');
  }
  if (v.note) {
    sub.push(v.note);
  }
  card.append(el('div', 'sub', sub.join('\n')));
  if (act.canEdit || v.email === me().email) {
    card.append(button('Edit', null, 'button button-small', () => openSignUp(act, role, v)));
    card.append(button('Remove', null, 'button button-secondary button-small', () => removeVolunteer(act, role, v)));
  }
  return card;
}

function volunteersSection(act, nodes) {
  const wrap = el('div');
  let any = false;
  for (const {role, node} of nodes) {
    if (!node.volunteers.length) {
      continue;
    }
    any = true;
    wrap.append(el('div', 'group-title', role ? role.title : `${act.title} (the activity itself)`));
    const grid = el('div', 'people-grid');
    for (const v of node.volunteers) {
      grid.append(personCard(act, role, v));
    }
    wrap.append(grid);
  }
  if (!any) {
    wrap.append(el('div', 'panel').appendChild(el('div', 'panel-empty', 'Nobody has signed up yet.')).parentNode);
  }
  if (act.volunteersHidden && !act.canEdit) {
    wrap.append(el('div', 'footnote', 'Sign-ups here are kept private; only the co-chairs see the full list.'));
  }
  return wrap;
}

function rolesSection(act, role, node, hiddenOnes) {
  const wrap = el('div');
  const head = el('div', 'list-head');
  const title = hiddenOnes ? 'Hidden and Pending' : (role ? 'Under this' : 'Committees, Tasks, and Roles');
  head.append(el('h2', 'section', title));
  const actions = el('div', 'row-actions');
  let query = '';
  const list = el('div');
  const roles = node.roles.filter(r => hiddenOnes ? r.status === 'Hidden' || r.status === 'Pending' : r.status !== 'Hidden' && r.status !== 'Pending');
  const render = () => {
    list.replaceChildren();
    const shown = roles.filter(r => matches(r, query));
    const groups = [];
    for (const r of shown) {
      const name = r.group || '';
      if (!groups.includes(name)) {
        groups.push(name);
      }
    }
    for (const name of groups) {
      if (name) {
        list.append(el('div', 'group-title', name));
      }
      const panel = el('div', 'panel');
      for (const r of shown.filter(x => (x.group || '') === name)) {
        panel.append(roleRow(act, r));
      }
      list.append(panel);
    }
    if (!shown.length) {
      const panel = el('div', 'panel');
      panel.append(el('div', 'panel-empty', hiddenOnes ? 'Nothing hidden or pending.' : 'No roles yet.'));
      list.append(panel);
    }
  };
  if (!hiddenOnes) {
    actions.append(searchBox('Search', q => {
      query = q;
      render();
    }, true));
    if (act.canEdit) {
      actions.append(button('Add a Role', 'plus', 'button', () => openRole(act, null, role)));
    } else if (act.status === 'Open') {
      actions.append(button('Suggest a Role', 'plus', 'button', () => openRole(act, null, role)));
    }
  }
  head.append(actions);
  wrap.append(head);
  render();
  wrap.append(list);
  return wrap;
}

function control(label, hint, input) {
  const wrap = el('label');
  wrap.append(input);
  const text = el('span');
  text.append(el('span', '', label));
  if (hint) {
    text.append(el('small', '', hint));
  }
  wrap.append(text);
  return wrap;
}

function checkbox(checked, onChange) {
  const input = el('input');
  input.type = 'checkbox';
  input.checked = checked;
  input.addEventListener('change', () => onChange(input.checked));
  return input;
}

function statusSelect(current, options, onChange) {
  const input = el('select');
  for (const status of options) {
    const option = el('option', '', status);
    option.value = status;
    option.selected = status === current;
    input.append(option);
  }
  input.addEventListener('change', () => onChange(input.value));
  return input;
}

function emailsOf(act, nodes) {
  const set = new Set();
  for (const {node} of nodes) {
    for (const v of node.volunteers) {
      set.add(v.email);
    }
  }
  return [...set].join(', ');
}

function editorBand(act, role, node, nodes) {
  const band = el('div', 'editor-band');
  band.append(el('h2', '', role ? 'Role Controls' : 'Admin Controls'));
  const controls = el('div', 'editor-controls');
  const save = changes => (role ? saveRoleFields(act, role, changes) : saveActivityFields(act, changes));
  const statuses = isAdmin() || role ? ['Pending', 'Open', 'Done', 'Hidden'] : ['Open', 'Done'];
  controls.append(control('Status', role ? 'Pending roles wait for approval; hidden ones only show here' : 'Open takes sign-ups; done and hidden do not',
    statusSelect(node.status, statuses, value => save({status: value}))));
  controls.append(control('Co-leader needed', 'Flag this as looking for someone to lead it', checkbox(node.coLeaderNeeded, on => save({coLeaderNeeded: on}))));
  controls.append(control('Hide volunteers', 'Check this if the sign-up list should stay private', checkbox(node.volunteersHidden, on => save({volunteersHidden: on}))));
  if (!role) {
    controls.append(control('Direct sign-up', 'People can join the activity itself, not only its roles', checkbox(act.directSignUp, on => save({directSignUp: on}))));
  }
  band.append(controls);
  const actions = el('div', 'editor-actions');
  actions.append(button('Copy Volunteer Emails', 'copy', 'button', () => copyText(emailsOf(act, nodes), 'Emails copied')));
  actions.append(button(role ? 'Edit Role' : 'Edit Activity', 'edit', 'button', () => (role ? openRole(act, role, null) : openActivity(act))));
  actions.append(button('Add Link', 'plus', 'button', () => openLink(act, role, null)));
  actions.append(button('Add a Role', 'plus', 'button', () => openRole(act, null, role)));
  if (!role && isAdmin() && act.year < years().next) {
    actions.append(button('Copy to Next Year', 'copy', 'button', () => copyToNextYear(act)));
  }
  band.append(actions);
  if (node.addedBy) {
    band.append(el('div', 'footnote', `Proposed by ${node.addedBy} on ${longDate(node.added)}`));
  }
  return band;
}

function calendarButton(node, act) {
  if (!parseWhen(node.start)) {
    return null;
  }
  const actions = el('div', 'center-actions');
  actions.append(button('Add to Calendar', 'calendar', 'button button-secondary', () => downloadCalendar(node, act)));
  return actions;
}

export function activityPage(act) {
  setTitle(act.title);
  const page = el('div');
  page.append(crumb([{label: act.year, href: `/years/${encodeURIComponent(act.year)}`}, {label: act.title}]));
  page.append(el('div', 'detail-year', act.year), header(act, act));
  const links = linksBand(act, null, act);
  if (links) {
    page.append(links);
  }
  const cal = calendarButton(act, act);
  if (cal) {
    page.append(cal);
  }
  const chairs = chairsSection(act);
  if (chairs) {
    page.append(chairs);
  }
  if (act.directSignUp || act.canEdit) {
    const band = signUpBand(act, null, act);
    if (band) {
      page.append(band);
    }
  }
  const nodes = [{role: null, node: act}, ...allRoles(act).map(r => ({role: r, node: r}))];
  const items = [{key: 'roles', label: 'Committees & Tasks'}, {key: 'volunteers', label: 'Volunteers'}];
  const hiddenRoles = allRoles(act).filter(r => r.status === 'Hidden' || r.status === 'Pending');
  if (act.canEdit && hiddenRoles.length) {
    items.push({key: 'hidden', label: `Hidden Things (${hiddenRoles.length})`});
  }
  let tab = 'roles';
  const body = el('div');
  const render = () => {
    body.replaceChildren(tabs(items, tab, key => {
      tab = key;
      render();
    }, true));
    if (tab === 'roles') {
      body.append(rolesSection(act, null, act, false));
    } else if (tab === 'volunteers') {
      body.append(volunteersSection(act, nodes));
    } else {
      body.append(rolesSection(act, null, {roles: hiddenRoles}, true));
    }
  };
  render();
  page.append(body);
  page.append(el('div', 'footnote', 'To remove yourself from a committee, open it and click Remove me.'));
  if (act.canEdit) {
    page.append(editorBand(act, null, act, nodes));
  }
  return page;
}

export function rolePage(act, role) {
  setTitle(role.title);
  const page = el('div');
  const parent = parentOf(act, role);
  const parts = [{label: act.year, href: `/years/${encodeURIComponent(act.year)}`}, {label: act.title, href: activityPath(act)}];
  if (parent) {
    parts.push({label: parent.title, href: rolePath(act, parent)});
  }
  parts.push({label: role.title});
  page.append(crumb(parts));
  page.append(el('div', 'detail-year', act.year));
  const up = link(parent ? rolePath(act, parent) : activityPath(act), 'row is-link panel');
  up.append(thumb((parent || act).imageUrl || act.imageUrl, (parent || act).title, 'small'));
  const upBody = el('div', 'row-body');
  upBody.append(el('div', 'label', parent ? 'Part of' : 'Activity'), el('div', 'row-title', (parent || act).title));
  up.append(upBody);
  const chevron = svg('chevron');
  chevron.classList.add('chevron');
  up.append(chevron);
  page.append(up, header(role, act));
  const links = linksBand(act, role, role);
  if (links) {
    page.append(links);
  }
  const cal = calendarButton(role, act);
  if (cal) {
    page.append(cal);
  }
  const band = signUpBand(act, role, role);
  if (band) {
    page.append(band);
  }
  const chairs = chairsSection(role);
  if (chairs) {
    page.append(chairs);
  }
  const nodes = [{role, node: role}];
  page.append(el('h2', 'section', 'Volunteers'), volunteersSection(act, nodes));
  if (role.roles.length || act.canEdit) {
    page.append(rolesSection(act, role, role, false));
    const hidden = role.roles.filter(r => r.status === 'Hidden' || r.status === 'Pending');
    if (act.canEdit && hidden.length) {
      page.append(rolesSection(act, role, {roles: hidden}, true));
    }
  }
  if (act.canEdit) {
    page.append(editorBand(act, role, role, nodes));
  }
  return page;
}
