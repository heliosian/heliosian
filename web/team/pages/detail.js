import {state, me, family, isAdmin, descendants, parentOf, rootOf, category, eventCategories, longDate, coChairs, mySignUp, canJoin, isFull, matches, activityPath, shownVolunteers, listHidden, listRevealed, canAdd, addLabel} from '../state.js';
import {badge, searchBox, treeFilter} from '../dom.js';
import {el, link, svg, imageThumb, avatar, button, editToggle, copyText, toast} from '/elements.js';
import {setTitle} from '/shell.js';
import {load, render} from '/router.js';
import {approvalButtons} from './approvals.js';
import {dateCard, googleCalendarLink, parseWhen} from '/datecard.js';
import {appOrigin} from '/appswitch.js';
import {childRow, categoryClass, completeBadge} from '../cards.js';
import {openPhotoLightbox} from '/crop.js';
import {heroImageBar} from '/heroimage.js';
import {personTile, peopleRow, offerTile, andList} from '/people.js';
import {api} from '/api.js';
import {openSignUp, openActivity, openLink, saveActivityFields, openPerson, openImageSearch, imageSearchOn, uploadAndSave, uploadImage, openVolunteerSettings, openVolunteerGrid, editPencil} from '../edit.js';

const phone = window.matchMedia('(max-width: 900px)');
phone.addEventListener('change', render);

const weekdayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short'});
const fullDate = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'});
const clock = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

function spansDays(node) {
  const start = parseWhen(node.start);
  const end = parseWhen(node.end);
  return Boolean(start && end && end.date.toDateString() !== start.date.toDateString());
}

function timeRange(node) {
  const start = parseWhen(node.start);
  if (!start || !start.hasTime) {
    return '';
  }
  const end = parseWhen(node.end);
  if (spansDays(node)) {
    const at = w => `${weekdayFormat.format(w.date)} ${clock.format(w.date)}`;
    return end.hasTime ? `${at(start)} – ${at(end)}` : at(start);
  }
  return end && end.hasTime ? `${clock.format(start.date)} – ${clock.format(end.date)}` : clock.format(start.date);
}

function heroStamp(act) {
  const start = parseWhen(act.start);
  if (act.timing || !start) {
    return act.timing ? el('div', 'hero-stamp hero-stamp-text', act.timing) : null;
  }
  const details = [act.description, location.origin + activityPath(act)].filter(Boolean).join('\n\n');
  return dateCard({start: act.start, end: act.end, location: act.location || '', add: googleCalendarLink({title: act.title, start: act.start, end: act.end, location: act.location || '', details})});
}

function sideCard(className) {
  return el('div', 'side-card ' + (className || ''));
}

function sideRow(icon, title, lines) {
  const row = el('div', 'side-row');
  const mark = el('div', 'side-icon');
  mark.append(svg(icon));
  row.append(mark);
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', title));
  for (const line of lines.filter(Boolean)) {
    body.append(el('div', 'side-line', line));
  }
  row.append(body);
  return row;
}

function factsCard(node, save) {
  const rows = [whenRow(node), chairsRow(node, save)].filter(Boolean);
  if (!rows.length) {
    return null;
  }
  const card = sideCard();
  card.append(...rows);
  return card;
}

function whenLines(node, start) {
  const when = [];
  if (node.timing) {
    when.push(node.timing);
  } else if (start) {
    when.push(spansDays(node) ? `${fullDate.format(start.date)} – ${fullDate.format(parseWhen(node.end).date)}` : fullDate.format(start.date));
    const hours = timeRange(node);
    if (hours) {
      when.push(hours);
    }
  }
  if (when.length && node.whenFrom) {
    when.push(`Same as ${node.whenFrom.title}`);
  }
  return when;
}

function whenRow(node) {
  const start = parseWhen(node.start);
  const when = whenLines(node, start);
  if (!(when.length && !start)) {
    return null;
  }
  return sideRow('calendar', 'Date & Time', when);
}

function chairsRow(node, save) {
  const chairs = coChairs(node);
  const options = shownVolunteers(node, node.canEdit).filter(v => v.position === 'Open to Co-Chair');
  if (!chairs.length && !node.coLeaderNeeded && !options.length && !node.canEdit) {
    return null;
  }
  const after = [];
  if (options.length) {
    const list = el('div', 'person-tiles');
    for (const v of options) {
      const tile = volunteerTile(node, v, false, false, true);
      list.append(node.canEdit ? offerTile(tile, makeChairButton(node, v)) : tile);
    }
    after.push(el('div', 'side-subtitle', 'Co-Chair Options'), list);
  }
  after.push(...coChairAsk(node, save));
  return peopleRow({
    title: chairs.length === 1 ? 'Chair' : 'Co-Chairs',
    line: chairs.length ? andList(chairs.map(v => v.name)) : 'Nobody yet.',
    tiles: chairs.map(v => volunteerTile(node, v, false)),
    after,
  });
}

function makeChairButton(node, v) {
  return button('Make co-chair', 'check', 'button button-small', async () => {
    if (!confirm(`Make ${v.name} a co-chair of ${node.title}? They will be able to edit the page and manage everyone who signs up.`)) {
      return;
    }
    try {
      await api('POST', '/api/team/volunteer', {id: node.id, email: v.email, position: 'Co-Chair', note: v.note || ''});
      await load();
      toast(`${v.name} is now a co-chair`);
    } catch (err) {
      toast(err.message);
    }
  });
}

function coChairAsk(node, save) {
  if (node.canEdit) {
    const ask = el('div', 'side-ask');
    const wants = el('label', 'side-switch');
    wants.append(checkbox(Boolean(node.coLeaderNeeded), on => save({coLeaderNeeded: on})), el('span', '', 'Still looking for a co-chair?'));
    const pencil = editPencil('Edit this page');
    pencil.addEventListener('click', () => openActivity(node));
    ask.append(wants, pencil);
    return [ask];
  }
  if (!node.coLeaderNeeded) {
    return [];
  }
  const mine = mySignUp(node);
  const nodes = [mine && mine.position === 'Open to Co-Chair'
    ? el('div', 'side-line side-offered', 'You have offered to co-chair - thank you!')
    : el('div', 'side-line side-need', 'A co-chair is needed - could that be you?')];
  if (!mine || mine.position === 'Volunteer') {
    nodes.push(button('Offer to Co-Chair', 'people', 'button button-small side-offer', async () => {
      try {
        await api('POST', '/api/team/volunteer', {id: node.id, position: 'Open to Co-Chair', note: mine ? mine.note : ''});
        await load();
        toast('Thank you - the organizers will be in touch.');
      } catch (err) {
        toast(err.message);
      }
    }));
  }
  return nodes;
}

function signedUpWords(owner, v) {
  const by = owner.canEdit ? v.addedByName : '';
  if (v.added) {
    return by ? `Signed up by ${by} on ${longDate(v.added)}` : `Signed up ${longDate(v.added)}`;
  }
  return by ? `Signed up by ${by}` : '';
}

function volunteerTile(owner, v, star, chair, option) {
  return personTile(v, {
    onClick: () => openPerson(v, owner),
    title: owner.canEdit ? `${v.name} and their sign-up` : `About ${v.name}`,
    name: v.name + (star && v.position === 'Co-Chair' ? '*' : ''),
    editable: owner.canEdit,
    role: chair ? 'chair' : option ? 'option' : '',
    rsvp: owner.canEdit ? v.rsvp : '',
    note: v.note,
    tip: signedUpWords(owner, v),
    gradeColors: state.model.gradeColors,
  });
}

function whereIs(root, node) {
  if (node === root) {
    return '(itself)';
  }
  const chain = [];
  for (let n = node; n && n !== root; n = parentOf(n)) {
    chain.unshift(n);
  }
  const top = chain[0];
  const cat = top && top.category ? category(top.category) : null;
  return [cat ? cat.title : null, ...chain.map(n => n.title)].filter(Boolean).join(' > ');
}

function chainBelow(root, node) {
  const titles = [];
  for (let n = node; n && n !== root; n = parentOf(n)) {
    titles.unshift(n.title);
  }
  return titles.join(' › ');
}

function familyBox(node) {
  const mine = [me().email, ...family().map(c => c.email)];
  const rows = [];
  for (const n of [node, ...descendants(node)]) {
    if (n.status === 'Hidden' || n.status === 'Pending') {
      continue;
    }
    for (const v of n.volunteers) {
      if (mine.includes(v.email)) {
        rows.push({n, v});
      }
    }
  }
  if (!rows.length) {
    return null;
  }
  const box = el('div', 'fam-box');
  box.append(el('h2', 'section section-swoosh', "My Family's Roles"));
  const list = el('div', 'fam-list');
  for (const {n, v} of rows) {
    const row = el('div', 'fam-row');
    row.append(avatar(v));
    const text = el('div', 'fam-text');
    const where = n === node ? n.title : chainBelow(node, n);
    const role = v.position === 'Co-Chair' ? 'Co-chair of ' : (v.position === 'Open to Co-Chair' ? 'Open to co-chairing ' : '');
    text.append(el('div', 'fam-name', v.name), el('div', 'fam-where', role + where));
    row.append(text);
    row.append(button('Edit', 'edit', 'link-button fam-edit', () => openSignUp(n, v)));
    list.append(row);
  }
  box.append(list);
  return box;
}

function volunteersBox(node) {
  const editing = Boolean(node.canEdit);
  const people = shownVolunteers(node, editing).filter(v => v.position !== 'Co-Chair');
  const chairs = coChairs(node);
  const revealed = listRevealed(node, editing);
  const view = {
    node,
    editing,
    people,
    chairs,
    revealed,
    showPeople: node.directSignUp && revealed ? people : [],
    somethingBelow: node.children.some(c => c.status !== 'Hidden' && c.status !== 'Pending'),
    quiet: !node.directSignUp && !chairs.length,
    listing: el('div', 'vol-listing'),
    filter: null,
  };
  const box = el('div', 'vol-box');
  if (view.quiet) {
    box.classList.add('is-quiet');
  }
  const below = descendants(node);
  if (below.length) {
    view.filter = treeFilter(node, below, () => paintVolunteers(view));
  }
  const mine = mySignUp(node);
  const head = el('div', 'vol-head');
  head.append(volunteersTitle(view), volunteersActions(node, view.filter, mine));
  box.append(head);
  paintVolunteers(view);
  box.append(view.listing);
  if (node.canEdit) {
    const roster = el('div', 'vol-roster');
    roster.append(button('Volunteer Info', 'list', 'button button-secondary button-small roster-open',
      () => openVolunteerGrid(node, [node, ...below], n => whereIs(node, n))));
    box.append(roster);
  }
  if (node.status === 'Open' && isFull(node) && !mine && node.directSignUp) {
    box.append(el('div', 'vol-note vol-full', node.volunteersComplete ? 'The volunteers are all set. Thank you, everyone!' : 'Every spot is taken. Thank you, everyone!'));
  }
  if (listHidden(node) && revealed) {
    box.append(el('div', 'vol-note', 'This list is private; only the organizers see it.'));
  }
  if (node.spots) {
    box.append(el('div', 'vol-note', `${node.taken} of ${node.spots} spots taken.`));
  }
  return box;
}

function volunteersTitle(view) {
  const title = el('div', 'vol-title');
  const count = view.chairs.length + view.people.length;
  if (!view.quiet) {
    title.append(el('h2', 'section section-swoosh', count ? `Who's Involved (${count})` : "Who's Involved"));
  } else if (view.somethingBelow) {
    title.append(el('div', 'vol-quiet', 'Sign up for something below.'));
  }
  return title;
}

function volunteersActions(node, filter, mine) {
  const actions = el('div', 'vol-actions');
  if (filter) {
    actions.append(filter.wrap);
  }
  if (node.directSignUp) {
    const signUp = signUpButton(node, mine);
    if (signUp) {
      actions.append(signUp);
    }
  }
  return actions;
}

function signUpButton(node, mine) {
  const self = mine
    ? {label: 'Edit my sign-up', icon: 'edit', onClick: () => openSignUp(node, mine)}
    : (canJoin(node) || node.canEdit ? {label: 'Join', icon: 'join', onClick: () => openSignUp(node, null)} : null);
  const others = node.canEdit || (node.status === 'Open' && !isFull(node));
  if (!others) {
    return self ? button(self.label, self.icon, 'button button-small', self.onClick) : null;
  }
  const split = el('div', 'split-button');
  const label = el('button', 'split-label');
  label.type = 'button';
  label.textContent = 'Sign up';
  label.addEventListener('click', () => (self ? self.onClick() : openSignUp(node, null, true)));
  split.append(label);
  if (self) {
    split.append(button(mine ? 'Edit mine' : 'Me', self.icon, 'split-segment', self.onClick));
  }
  split.append(button('Someone else', 'plus', 'split-segment', () => openSignUp(node, null, true)));
  return split;
}

function paintVolunteers(view) {
  const {node, listing, filter} = view;
  listing.replaceChildren();
  const sources = filter ? filter.sources() : [node];
  const others = sources.filter(n => n !== node);
  if (sources.includes(node)) {
    ownVolunteers(view, others.length > 0);
  }
  for (const src of others) {
    listing.append(el('div', 'side-group', chainBelow(node, src)));
    const theirs = shownVolunteers(src, view.editing);
    if (!theirs.length) {
      listing.append(el('div', 'vol-note', 'Nobody yet.'));
      continue;
    }
    const grid = el('div', 'person-tiles vol-people');
    for (const v of theirs) {
      grid.append(volunteerTile(src, v, true));
    }
    listing.append(grid);
  }
  if (!sources.length) {
    listing.append(el('div', 'vol-note', 'Nothing selected.'));
  }
}

function ownVolunteers(view, labelled) {
  const {node, listing, editing, chairs, people, showPeople} = view;
  if (chairs.length || showPeople.length) {
    const grid = el('div', 'person-tiles vol-people');
    for (const v of chairs) {
      grid.append(volunteerTile(node, v, false, true));
    }
    const isOption = v => v.position === 'Open to Co-Chair';
    for (const v of showPeople.filter(isOption)) {
      grid.append(volunteerTile(node, v, false, false, true));
    }
    for (const v of showPeople.filter(v => !isOption(v))) {
      grid.append(volunteerTile(node, v, false));
    }
    if (labelled) {
      listing.append(el('div', 'side-group', view.filter.self));
    }
    listing.append(grid);
  }
  if (!node.directSignUp) {
    if (!view.quiet && view.somethingBelow) {
      listing.append(el('div', 'vol-note', 'Sign up for something below.'));
    }
    if (editing && people.length) {
      const n = people.length;
      listing.append(el('div', 'vol-note vol-held',
        `${n} ${n === 1 ? 'person' : 'people'} signed up here before volunteers were turned off. Check Allow volunteers for this itself to show them again.`));
    }
    return;
  }
  if (!view.revealed) {
    listing.append(el('div', 'vol-note', 'This list is private; only the organizers see it.'));
  } else if (!people.length && !chairs.length) {
    listing.append(el('div', 'vol-note', 'Nobody yet.'));
  }
}

function priorityCard(node, save) {
  if (!isAdmin() || isFull(node)) {
    return null;
  }
  const card = sideCard('priority-card');
  const on = Boolean(node.priority);
  card.append(button(on ? 'Remove High Priority' : 'Mark High Priority', 'star', 'button button-small ' + (on ? 'button-secondary' : '') + ' priority-button', () => save({priority: !on})));
  card.append(el('div', 'side-line', on ? 'Listed under High Priority here and on Heliosian\u2019s front page.' : 'Lists it under High Priority here and on Heliosian\u2019s front page.'));
  return card;
}

function resourcesCard(node) {
  const editing = Boolean(node.canEdit);
  if (!node.links.length && !editing) {
    return null;
  }
  const card = sideCard();
  const links = el('div', 'side-links');
  card.append(links);
  for (const item of node.links) {
    const row = link(item.url, 'side-link');
    row.removeAttribute('data-link');
    row.target = '_blank';
    row.rel = 'noopener';
    row.append(imageThumb(item.imageUrl || node.imageUrl || rootOf(node).imageUrl, item.title, 'side-link-thumb'));
    const body = el('div', 'side-row-body');
    body.append(el('div', 'side-link-title', item.title));
    body.append(el('div', 'side-link-desc', item.description || item.url.replace(/^https?:\/\//, '').replace(/\/$/, '')));
    row.append(body, svg('open'));
    if (!editing) {
      links.append(row);
      continue;
    }
    const line = el('div', 'side-link-row');
    const edit = el('button', 'icon-round side-link-edit');
    edit.type = 'button';
    edit.title = `Edit “${item.title}”`;
    edit.setAttribute('aria-label', edit.title);
    edit.append(svg('edit'));
    edit.addEventListener('click', () => openLink(node, item));
    line.append(row, edit);
    links.append(line);
  }
  if (editing) {
    card.append(button('Add a Resource', 'plus', 'button button-secondary button-small side-button',
      () => openLink(node, null)));
  }
  return card;
}

function highlightCard(node) {
  const h = node.highlight;
  if (!h) {
    return null;
  }
  const card = el('div', 'highlight-card');
  if (h.icon) {
    card.append(el('div', 'highlight-icon', h.icon));
  }
  const body = el('div', 'highlight-body');
  if (h.headline) {
    body.append(el('div', 'highlight-headline', h.headline));
  }
  for (const para of (h.body || '').split(/\n\s*\n/).filter(t => t.trim())) {
    const p = el('p', 'highlight-text');
    para.trim().split(/(\*\*[^*]+\*\*)/).forEach(part => {
      const bold = part.match(/^\*\*([^*]+)\*\*$/);
      p.append(bold ? el('strong', '', bold[1]) : part);
    });
    body.append(p);
  }
  card.append(body);
  return card;
}

function flyerCard(node, save) {
  if (!node.flyerUrl && !node.canEdit) {
    return null;
  }
  const card = sideCard('flyer-card');
  const head = el('div', 'flyer-head');
  head.append(el('div', 'side-title', 'Flyer'));
  card.append(head);
  if (node.flyerUrl) {
    const open = el('button', 'flyer-open');
    open.type = 'button';
    open.title = 'View full size';
    const img = el('img', 'flyer-image');
    img.src = node.flyerUrl;
    img.alt = `${node.title} flyer`;
    open.append(img);
    open.addEventListener('click', () => openPhotoLightbox(node.flyerUrl));
    card.append(open);
  } else {
    card.append(el('div', 'side-line', 'No flyer yet.'));
  }
  if (node.canEdit) {
    const bar = el('div', 'flyer-actions');
    const file = el('input');
    file.type = 'file';
    file.accept = 'image/*';
    file.hidden = true;
    file.addEventListener('change', async () => {
      if (file.files.length) {
        await uploadAndSave(changes => save({flyer: changes.image}), file.files[0]);
      }
    });
    const upload = el('label', 'button button-secondary button-small');
    upload.append(svg('up'), el('span', '', node.flyer ? 'Replace' : 'Upload'), file);
    bar.append(upload);
    if (node.flyer) {
      bar.append(button('Remove', 'trash', 'button button-secondary button-small', () => save({flyer: ''})));
    }
    card.append(bar);
  }
  return card;
}

function emailListPath(node) {
  if (node.emailList) {
    return appOrigin('loop') + '/groups/' + encodeURIComponent(node.emailList);
  }
  return appOrigin('loop') + '/new?from=' + encodeURIComponent('activity:' + node.id);
}

function emailListWords(node) {
  return node.emailList ? `${node.title} Email List` : `Create ${node.title} Email List`;
}

function emailListCard(node) {
  if (!node.runs) {
    return null;
  }
  const card = sideCard('side-card-invite');
  const address = node.emailList ? `${node.emailList}@loop.heliosian.com` : '';
  const [title, words] = node.emailList
    ? [`${node.title} Email List`, `${address} reaches everyone signed up for ${node.title}, and its co-chairs, as people join and leave.`]
    : [`Email everyone in ${node.title}`, `Make an email list on Helios Loop of everyone signed up for ${node.title}, and its co-chairs. It keeps up as people join and leave.`];
  card.append(el('div', 'side-title', title), el('div', 'side-line', words));
  const a = el('a', 'button button-small side-button');
  a.href = emailListPath(node);
  a.append(svg('mail'), el('span', '', emailListWords(node)));
  card.append(a);
  return card;
}

function helpCard(node) {
  const chairs = coChairs(node).filter(v => v.email);
  if (!chairs.length) {
    return null;
  }
  const card = sideCard('side-card-help');
  card.append(el('div', 'side-title', 'Need help?'));
  card.append(el('p', 'side-line', `Have a question about ${node.title}? Ask whoever is running it.`));
  const mail = el('a', 'button button-secondary button-small side-button');
  mail.href = `mailto:${chairs.map(v => v.email).join(',')}?subject=${encodeURIComponent(node.title)}`;
  mail.append(svg('mail'), el('span', '', chairs.length === 1 ? 'Contact the Chair' : 'Contact the Co-Chairs'));
  card.append(mail);
  return card;
}

function heroButton(icon, label, onClick) {
  const node = el('button', 'hero-action');
  node.type = 'button';
  node.title = label;
  node.setAttribute('aria-label', label);
  node.append(svg(icon));
  node.addEventListener('click', onClick);
  return node;
}

function shareButton(node) {
  return heroButton('share', 'Share this page', async () => {
    const url = location.origin + activityPath(node);
    if (navigator.share) {
      try {
        await navigator.share({title: node.title, url});
        return;
      } catch {
      }
    }
    copyText(url, 'Link copied');
  });
}

function childrenSection(node) {
  const editing = Boolean(node.canEdit);
  const root = rootOf(node);
  const unlisted = r => r.status === 'Hidden' || r.status === 'Pending' || Boolean(r.categoryHidden);
  const roles = node.children.filter(r => !unlisted(r) || editing);
  if (!roles.length && (node.parent || !eventCategories(root).length)) {
    return node.canEdit ? addActivityBar(node) : null;
  }
  const section = {node, root, roles, editing, query: '', reordering: false, list: el('div')};
  const head = el('div', 'list-head');
  head.append(el('h2', 'section section-swoosh', 'Opportunities & Needs'));
  const actions = el('div', 'row-actions');
  if (roles.length >= 10) {
    actions.append(searchBox('Search', q => {
      section.query = q;
      paintChildren(section);
    }, true));
  }
  head.append(actions);
  const wrap = el('div');
  wrap.append(head);
  paintChildren(section);
  wrap.append(section.list);
  return wrap;
}

function settingsButton(node) {
  return button('Volunteer settings', 'gear', 'button button-secondary button-small', () => openVolunteerSettings(node));
}

function addActivityBar(node) {
  const bar = el('div', 'add-row');
  bar.append(button('Add Activity', 'plus', 'button button-secondary button-small', () => openActivity(null, {parent: node, category: ''})), settingsButton(node));
  return bar;
}

function groupHead(node, cat, others) {
  const row = el('div', 'group-row');
  row.append(el('div', 'group-title', cat ? cat.title : (others ? 'Uncategorized' : '')));
  const open = () => openActivity(null, {parent: node, category: cat ? cat.id : ''});
  let add = null;
  const policy = cat || node;
  if (node.canEdit) {
    add = button('Add', 'plus', 'button button-secondary button-small', open);
  } else if (node.status === 'Open' && canAdd(policy)) {
    add = button(addLabel(policy), 'plus', 'button button-secondary button-small', open);
  }
  const tools = el('div', 'group-tools');
  if (add) {
    add.title = cat ? `Add to ${cat.title}` : 'Add without a category';
    tools.append(add);
  }
  row.append(tools);
  return row;
}

async function reorderChildren(section, id, before, categoryId) {
  const {node, list} = section;
  if (section.reordering) {
    return;
  }
  section.reordering = true;
  list.classList.add('is-reordering');
  const ids = node.children.map(c => c.id).filter(x => x !== id);
  let at = before ? ids.indexOf(before) : -1;
  if (at < 0) {
    at = ids.length;
    for (let i = ids.length - 1; i >= 0; i--) {
      const c = node.children.find(x => x.id === ids[i]);
      if ((c.category || '') === categoryId) {
        at = i + 1;
        break;
      }
    }
  }
  ids.splice(at, 0, id);
  try {
    await api('POST', '/api/team/order', {parent: node.id, ids});
    await load();
  } catch (err) {
    toast(err.message);
  } finally {
    section.reordering = false;
    list.classList.remove('is-reordering');
  }
}

function dropTarget(section, panel, categoryId) {
  if (!section.editing) {
    return panel;
  }
  panel.addEventListener('dragover', e => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    panel.classList.add('is-dragover');
  });
  panel.addEventListener('dragleave', () => panel.classList.remove('is-dragover'));
  panel.addEventListener('drop', e => {
    e.preventDefault();
    panel.classList.remove('is-dragover');
    const moved = section.roles.find(r => r.id === e.dataTransfer.getData('text/plain'));
    if (!moved) {
      return;
    }
    if ((moved.category || '') !== categoryId) {
      saveActivityFields(moved, {category: categoryId});
      return;
    }
    const over = e.target.closest('.row');
    const before = over && over.dataset.id !== moved.id ? over.dataset.id : '';
    if (before || !over) {
      reorderChildren(section, moved.id, before, categoryId);
    }
  });
  return panel;
}

function sectionRow(section, r) {
  const row = childRow(r, section.editing);
  row.dataset.id = r.id;
  if (section.editing) {
    row.addEventListener('dragover', e => {
      e.preventDefault();
      row.classList.add('is-dropbefore');
    });
    row.addEventListener('dragleave', () => row.classList.remove('is-dropbefore'));
    row.addEventListener('drop', () => row.classList.remove('is-dropbefore'));
  }
  return row;
}

function paintChildren(section) {
  const {node, root, list, query} = section;
  list.replaceChildren();
  const shown = section.roles.filter(r => matches(r, query));
  const order = [...eventCategories(root).map(c => c.id), ''];
  const present = new Set(shown.map(r => r.category || ''));
  const others = node.parent ? [...present].some(Boolean) : eventCategories(root).length > 0;
  for (const id of order) {
    if (!present.has(id)) {
      continue;
    }
    list.append(groupHead(node, id ? category(id) : null, others));
    const panel = dropTarget(section, el('div', 'panel'), id);
    for (const r of shown.filter(x => (x.category || '') === id)) {
      panel.append(sectionRow(section, r));
    }
    list.append(panel);
  }
  if (!query && !node.parent) {
    emptyCategories(section, present);
  }
  if (!shown.length) {
    if (!eventCategories(root).length || node.parent) {
      list.append(groupHead(node, null, false));
    }
    const panel = el('div', 'panel');
    panel.append(el('div', 'panel-empty', query ? 'Nothing matches.' : 'Nothing here yet.'));
    list.append(panel);
  }
  const first = list.querySelector('.group-tools');
  if (node.canEdit && first) {
    first.prepend(settingsButton(node));
  }
}

function emptyCategories(section, present) {
  for (const c of eventCategories(section.root)) {
    if (present.has(c.id)) {
      continue;
    }
    section.list.append(groupHead(section.node, c));
    if (section.editing) {
      const panel = dropTarget(section, el('div', 'panel is-drop-hint'), c.id);
      panel.append(el('div', 'panel-empty', 'Drag something here'));
      section.list.append(panel);
    }
  }
}

function checkbox(checked, onChange) {
  const input = el('input');
  input.type = 'checkbox';
  input.checked = checked;
  input.addEventListener('change', () => onChange(input.checked));
  return input;
}

function editorBand(node) {
  const band = el('div', 'editor-band');
  const when = node.added ? ` on ${longDate(node.added)}` : '';
  band.append(el('div', 'footnote', `Proposed by ${node.addedBy}${when}`));
  return band;
}

export function activityPage(node) {
  const root = rootOf(node);
  const parent = parentOf(node);
  const save = changes => saveActivityFields(node, changes);
  setTitle(node.title);
  const page = el('div', 'detail');

  const top = el('div', 'detail-top');
  const back = link(parent ? activityPath(parent) : '/', 'detail-back');
  back.append(svg('chevron-left'), el('span', '', parent ? `Back to ${parent.title}` : 'Back to Opportunities'));
  top.append(back);
  if (node.canEdit) {
    const tools = el('div', 'detail-tools');
    tools.append(settingsButton(node), editToggle(false, () => openActivity(node)));
    top.append(tools);
  }
  page.append(top);

  const hero = el('div', 'detail-hero');
  hero.append(imageThumb(node.imageUrl || root.imageUrl, node.title, 'detail-hero-image ' + categoryClass(root.category)));
  const stamp = heroStamp(node);
  if (stamp) {
    hero.append(stamp);
  }
  const heroActions = el('div', 'hero-actions');
  heroActions.append(shareButton(node));
  if (node.imageUrl) {
    heroActions.append(heroButton('expand', 'View full size', () => openPhotoLightbox(node.imageUrl)));
  }
  hero.append(heroActions);
  if (node.canEdit) {
    hero.append(heroImageBar({image: node.image, imageUrl: node.imageUrl, query: node.title, tools: {uploadImage, imageSearchOn, openImageSearch}, save: image => save({image})}));
  }
  page.append(hero);

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const marks = el('div', 'detail-marks');
  if (parent) {
    marks.append(link(activityPath(parent), 'card-chip is-inline is-link ' + categoryClass(root.category), parent.title));
  } else if (category(node.category)) {
    marks.append(el('span', 'card-chip is-inline ' + categoryClass(node.category), category(node.category).title));
  }
  if (node.coLeaderNeeded) {
    marks.append(el('span', 'need', 'Co-chair needed!'));
  }
  if (node.status !== 'Open') {
    marks.append(badge(node.status === 'Pending' ? 'Needs approval' : node.status, node.status.toLowerCase()));
    if (node.status === 'Pending' && isAdmin()) {
      marks.append(...approvalButtons(node));
    }
  } else if (isFull(node)) {
    marks.append(completeBadge());
  }
  if (marks.children.length) {
    main.append(marks);
  }
  const heading = el('div', 'detail-heading');
  heading.append(el('h1', 'detail-title', node.title));
  main.append(heading);
  const blurb = el('div', 'detail-blurb');
  if (node.description) {
    for (const para of node.description.split(/\n\s*\n/).filter(t => t.trim())) {
      blurb.append(el('p', 'detail-text', para.trim()));
    }
  }
  if (blurb.children.length) {
    main.append(blurb);
  }

  const highlight = highlightCard(node);
  if (highlight) {
    main.append(highlight);
  }
  const resources = resourcesCard(node);
  if (resources) {
    resources.classList.add('resources-main');
    main.append(resources);
  }
  const facts = factsCard(node, save);
  if (facts && phone.matches) {
    facts.classList.add('facts-inline');
    main.append(facts);
  }

  const under = node.children.filter(c => c.status !== 'Hidden' && c.status !== 'Pending');
  const familyRoles = familyBox(node);
  if (familyRoles) {
    main.append(familyRoles);
  }
  main.append(volunteersBox(node));
  if (!parent || under.length || node.canEdit) {
    const things = childrenSection(node);
    if (things) {
      main.append(things);
    }
  }
  if (!parent && under.length) {
    main.append(el('div', 'footnote', 'To leave a committee, open it and use Edit my sign-up.'));
  }
  if (node.canEdit && node.addedBy) {
    main.append(editorBand(node));
  }

  const side = el('aside', 'detail-side');
  for (const card of [emailListCard(node), phone.matches ? null : facts, flyerCard(node, save), helpCard(node), priorityCard(node, save)]) {
    if (card) {
      side.append(card);
    }
  }
  cols.append(main, side);
  page.append(cols);
  return page;
}
