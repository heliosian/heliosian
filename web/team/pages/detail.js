import {state, me, family, isAdmin, isSystemAdmin, years, allYears, descendants, parentOf, rootOf, category, eventCategories, longDate, coChairs, mySignUp, canJoin, isFull, matches, activityPath, listedIn, sortByStart, shiftedEnd, headingChoices, shownVolunteers, listHidden, listRevealed, canAdd, addLabel, ADDING} from '../state.js';
import {el, link, svg, thumb, avatar, badge, button, searchBox, copyText, whenEditor, toast} from '../dom.js';
import {setTitle} from '/shell.js';
import {approvalButtons} from './approvals.js';
import {dateCard, googleCalendarLink, parseWhen} from '/datecard.js';
import {appOrigin} from '/toolbar.js';
import {childRow, categoryClass, completeBadge} from '../cards.js';
import {openCropTool, openPhotoLightbox} from '/crop.js';
import {send, reload, openSignUp, openActivity, openLink, saveActivityFields, openPerson, openImageSearch, imageSearchOn, editable, fieldEditor, highlightInputs, uploadAndSave, openCategoryManager, openVolunteerGrid, editPencil} from '../edit.js';
import {text as textInput, textarea as textAreaInput, select as selectInput} from '/form.js';


let editingPath = null;

const phone = window.matchMedia('(max-width: 900px)');
phone.addEventListener('change', () => document.dispatchEvent(new CustomEvent('hca:refresh')));

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
  return dateCard(el, {start: act.start, end: act.end, location: act.location || '', add: googleCalendarLink({title: act.title, start: act.start, end: act.end, location: act.location || '', details})});
}

function heroImageBar(node, save) {
  const bar = el('div', 'hero-image-bar');
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  file.addEventListener('change', async () => {
    if (!file.files.length) {
      return;
    }
    bar.replaceChildren(el('span', 'hero-image-status', 'Uploading…'));
    await uploadAndSave(save, file.files[0]);
  });
  const label = node.image ? 'Replace image' : 'Add an image';
  if (imageSearchOn()) {
    const holder = el('div', 'hero-image-menu-holder');
    const toggle = el('button', 'hero-image-action');
    toggle.type = 'button';
    toggle.setAttribute('aria-haspopup', 'menu');
    toggle.append(svg('image'), el('span', '', label), svg('caret'));
    const menu = el('div', 'hero-image-menu');
    menu.hidden = true;
    const item = (icon, text, onClick) => {
      const b = el('button', 'hero-image-menu-item');
      b.type = 'button';
      b.append(svg(icon), el('span', '', text));
      b.addEventListener('click', () => {
        menu.hidden = true;
        onClick();
      });
      menu.append(b);
    };
    item('up', 'Upload image', () => file.click());
    item('search', 'Find an image', () => openImageSearch(node.title, picked => save({image: picked.name})));
    toggle.addEventListener('click', e => {
      e.stopPropagation();
      menu.hidden = !menu.hidden;
    });
    document.addEventListener('click', () => {
      menu.hidden = true;
    }, {once: true, capture: true});
    holder.append(toggle, menu, file);
    bar.append(holder);
  } else {
    const choose = el('label', 'hero-image-action');
    choose.append(svg('image'), el('span', '', label), file);
    bar.append(choose);
  }
  if (node.image) {
    const crop = el('button', 'hero-image-action');
    crop.type = 'button';
    crop.append(svg('crop'), el('span', '', 'Crop'));
    crop.addEventListener('click', () => openCropTool(node.imageUrl, false, async blob => {
      await uploadAndSave(save, new File([blob], 'crop.jpg', {type: 'image/jpeg'}));
      return true;
    }));
    bar.append(crop);
    const remove = el('button', 'hero-image-action');
    remove.type = 'button';
    remove.append(svg('trash'), el('span', '', 'Remove'));
    remove.addEventListener('click', () => save({image: ''}));
    bar.append(remove);
  }
  return bar;
}

function prevNext(node) {
  const parent = parentOf(node);
  const siblings = parent
    ? parent.children.filter(c => c.status !== 'Hidden' && c.status !== 'Pending')
    : sortByStart(listedIn(node.year));
  const i = siblings.findIndex(a => a.title === node.title);
  const nav = el('div', 'detail-jump');
  const make = (target, label, icon, cls) => {
    if (!target) {
      const dead = el('span', 'detail-jump-link is-off');
      dead.append(icon === 'back' ? svg('back') : el('span', '', label));
      dead.append(icon === 'back' ? el('span', '', label) : svg('chevron'));
      return dead;
    }
    const a = link(activityPath(target), 'detail-jump-link');
    a.title = target.title;
    if (icon === 'back') {
      a.append(svg('back'), el('span', '', label));
    } else {
      a.append(el('span', '', label), svg('chevron'));
    }
    return a;
  };
  nav.append(make(i > 0 ? siblings[i - 1] : null, 'Previous', 'back'));
  nav.append(make(i >= 0 && i < siblings.length - 1 ? siblings[i + 1] : null, 'Next', 'next'));
  return nav;
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

function factsCard(node, editing, save) {
  const card = sideCard();
  let any = false;
  const start = parseWhen(node.start);
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
  if ((when.length && !start) || editing) {
    const row = sideRow('calendar', 'Date & Time', when.length ? when : ['Not scheduled']);
    const body = row.querySelector('.side-row-body');
    if (editing) {
      const lines = el('div', 'side-when');
      while (body.children.length > 1) {
        lines.append(body.children[1]);
      }
      body.append(lines);
      body.querySelector('.side-title').append(editable(lines, 'Edit when this happens',
        () => {
          const when = whenEditor(node.own.start, node.own.end, node.own.timing, parentOf(node));
          when.wrap.focus = when.focus;
          return {input: when.wrap, value: when.value, validate: when.validate};
        },
        value => save(value)));
    }
    card.append(row);
    any = true;
  }
  const chairs = coChairs(node);
  const options = shownVolunteers(node, node.canEdit).filter(v => v.position === 'Open to Co-Chair');
  if (chairs.length || node.coLeaderNeeded || options.length || editing) {
    const row = el('div', 'side-row');
    const mark = el('div', 'side-icon');
    mark.append(svg('people'));
    row.append(mark);
    const body = el('div', 'side-row-body');
    body.append(el('div', 'side-title', chairs.length === 1 ? 'Chair' : 'Co-Chairs'));
    if (chairs.length) {
      const list = el('div', 'side-chairs');
      for (const v of chairs) {
        list.append(personTile(node, v, editing, false));
      }
      body.append(list);
    } else {
      body.append(el('div', 'side-line', 'Nobody yet.'));
    }
    if (options.length) {
      body.append(el('div', 'side-subtitle', 'Co-Chair Options'));
      if (node.canEdit) {
        for (const v of options) {
          const card = el('div', 'side-option');
          const who = el('button', 'side-option-who');
          who.type = 'button';
          who.title = `${v.name} and their sign-up`;
          who.addEventListener('click', () => openPerson(v, node));
          const text = el('div', 'side-option-text');
          text.append(el('div', 'side-option-name', v.name), el('div', 'side-option-hint', 'Offered to co-chair'));
          who.append(avatar(v), text);
          card.append(who);
          card.append(button('Make co-chair', 'check', 'button button-small side-approve', async () => {
            if (!confirm(`Make ${v.name} a co-chair of ${node.title}? They will be able to edit the page and manage everyone who signs up.`)) {
              return;
            }
            try {
              await send('POST', '/api/team/volunteer', {id: node.id, email: v.email, position: 'Co-Chair', note: v.note || ''});
              await reload();
              toast(`${v.name} is now a co-chair`);
            } catch (err) {
              toast(err.message);
            }
          }));
          body.append(card);
        }
      } else {
        const list = el('div', 'side-chairs');
        for (const v of options) {
          list.append(personTile(node, v, false, false, false, true));
        }
        body.append(list);
      }
    }
    if (editing) {
      const wants = el('label', 'side-switch');
      const box = checkbox(node.coLeaderNeeded, on => save({coLeaderNeeded: on}));
      wants.append(box, el('span', '', 'A co-chair is needed'));
      body.append(wants);
    } else if (node.coLeaderNeeded && node.canEdit) {
      const ask = el('div', 'side-ask');
      const wants = el('label', 'side-switch');
      const box = checkbox(true, on => save({coLeaderNeeded: on}));
      wants.append(box, el('span', '', 'Still looking for a co-chair?'));
      const pencil = editPencil('Edit this page');
      pencil.addEventListener('click', () => {
        editingPath = node.id;
        document.dispatchEvent(new CustomEvent('hca:refresh'));
      });
      ask.append(wants, pencil);
      body.append(ask);
    } else if (node.coLeaderNeeded) {
      const mine = mySignUp(node);
      if (mine && mine.position === 'Open to Co-Chair') {
        body.append(el('div', 'side-line side-offered', 'You have offered to co-chair - thank you!'));
      } else {
        body.append(el('div', 'side-line side-need', 'A co-chair is needed - could that be you?'));
      }
      if (!mine || mine.position === 'Volunteer') {
        body.append(button('Offer to Co-Chair', 'people', 'button button-small side-offer', async () => {
          try {
            await send('POST', '/api/team/volunteer', {id: node.id, position: 'Open to Co-Chair', note: mine ? mine.note : ''});
            await reload();
            toast('Thank you - the organizers will be in touch.');
          } catch (err) {
            toast(err.message);
          }
        }));
      }
    }
    row.append(body);
    card.append(row);
    any = true;
  }
  return any ? card : null;
}

function personTile(owner, v, editing, star, chair, option) {
  const tile = el('button', 'side-chair' + (editing ? ' is-editable' : '') + (chair ? ' is-chair' : '') + (option ? ' is-option' : ''));
  tile.type = 'button';
  tile.title = owner.canEdit ? `${v.name} and their sign-up` : `About ${v.name}`;
  tile.addEventListener('click', () => openPerson(v, owner));
  const face = avatar(v);
  if (v.grade) {
    const grade = el('span', 'grade-badge', /^kindergarten$/i.test(v.grade) ? 'K' : v.grade.replace(/^grade\s*/i, ''));
    grade.title = v.grade;
    const color = (state.model.gradeColors || {})[v.grade];
    if (color) {
      grade.style.background = `color-mix(in srgb, ${color} 65%, black)`;
    }
    face.append(grade);
  }
  if (v.note) {
    const bubble = el('span', 'note-badge');
    bubble.setAttribute('aria-label', 'Left a note');
    bubble.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="5" y="3" width="14" height="18" rx="2.5" fill="currentColor"/><path class="note-ink" d="M5 5.5A2.5 2.5 0 0 1 7.5 3h9A2.5 2.5 0 0 1 19 5.5V7.5H5z"/><rect class="note-ink" x="8" y="10.5" width="8" height="1.6" rx="0.8"/><rect class="note-ink" x="8" y="14" width="8" height="1.6" rx="0.8"/><rect class="note-ink" x="8" y="17.5" width="5" height="1.6" rx="0.8"/></svg>';
    face.append(bubble);
  }
  tile.append(face, el('div', 'side-chair-name', v.name + (star && v.position === 'Co-Chair' ? '*' : '')));
  if (chair) {
    tile.append(el('div', 'side-chair-role', 'Chair'));
  } else if (option) {
    tile.append(el('div', 'side-chair-role is-option', 'Chair opt'));
  }
  if (v.rsvp && owner.canEdit) {
    const words = {yes: 'RSVP: Yes', maybe: 'RSVP: Maybe', no: 'RSVP: No', none: 'No RSVP yet'};
    tile.append(el('div', 'side-chair-rsvp is-' + v.rsvp, words[v.rsvp] || ''));
  }
  const tip = el('div', 'tile-tip');
  tip.setAttribute('role', 'tooltip');
  const by = owner.canEdit ? v.addedByName : '';
  if (v.added) {
    tip.append(el('div', '', by ? `Signed up by ${by} on ${longDate(v.added)}` : `Signed up ${longDate(v.added)}`));
  } else if (by) {
    tip.append(el('div', '', `Signed up by ${by}`));
  }
  if (v.note) {
    tip.append(el('div', 'tile-tip-note', `“${v.note}”`));
  }
  if (tip.childElementCount) {
    tile.append(tip);
  }
  return tile;
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

function editCrumb(node, parent, root, save) {
  const crumb = el('div', 'detail-crumb');
  const yearItem = el('span', 'crumb-item', node.year);
  crumb.append(yearItem);
  if (isAdmin() && !parent) {
    crumb.append(editable(yearItem, 'Change the school year',
      () => {
        const options = allYears();
        for (const y of [years().current, years().next]) {
          if (!options.includes(y)) {
            options.unshift(y);
          }
        }
        const input = selectInput(options, node.year);
        return {input, hint: 'Everything under it moves with it.', value: () => input.value};
      },
      value => save({year: value})));
  }
  if (parent) {
    crumb.append(el('span', 'crumb-sep', '›'));
    crumb.append(link(activityPath(parent), 'crumb-item crumb-link', parent.title));
  }
  crumb.append(el('span', 'crumb-sep', '›'));
  const current = category(node.category);
  const catItem = el('span', 'crumb-item' + (current ? '' : ' is-empty'), current ? current.title : 'No category');
  crumb.append(catItem);
  crumb.append(editable(catItem, 'Change category',
    () => {
      const REPARENT = '\u0000reparent';
      const choices = parent
        ? [{label: 'No category', value: ''}, ...eventCategories(root).map(c => ({label: c.title, value: c.id}))]
        : headingChoices();
      if (!parent) {
        choices.push({label: 'Change Parent Event…', value: REPARENT});
      }
      const wrap = el('div', 'field-when');
      const pick = selectInput(choices, node.category || '');
      wrap.append(pick);
      const mine = new Set([node.id, ...descendants(node).map(d => d.id)]);
      const candidates = [];
      for (const top of state.model.activities.filter(a => a.year === node.year)) {
        for (const n of [top, ...descendants(top)]) {
          if (mine.has(n.id)) {
            continue;
          }
          const titles = [];
          for (let x = n; x; x = parentOf(x)) {
            titles.unshift(x.title);
          }
          candidates.push({label: titles.join(' › '), value: n.id});
        }
      }
      const target = selectInput(candidates, candidates[0] ? candidates[0].value : '');
      const targetLabel = el('span', 'field-when-label', 'Put this under');
      targetLabel.hidden = target.hidden = true;
      wrap.append(targetLabel, target);
      pick.addEventListener('change', () => {
        targetLabel.hidden = target.hidden = pick.value !== REPARENT;
      });
      wrap.focus = () => pick.focus();
      return {
        input: wrap,
        hint: parent ? '' : 'Or make this part of another event; it drops its category and lists under that event instead.',
        value: () => (pick.value === REPARENT ? {parent: target.value, category: ''} : {category: pick.value}),
        validate: v => (v.parent === '' && pick.value === REPARENT ? 'Pick the event to put this under.' : ''),
      };
    },
    value => save(value)));
  return crumb;
}

function treeFilter(node, below, onChange) {
  const chosen = new Set([node.id]);
  const self = `${node.title} (itself)`;
  const filter = el('div', 'side-filter');
  const toggle = el('button', 'side-filter-toggle');
  toggle.type = 'button';
  const summary = el('span', '', self);
  toggle.append(summary, svg('caret'));
  const menu = el('div', 'side-filter-menu');
  menu.hidden = true;
  const depthOf = n => {
    let d = 0;
    for (let p = parentOf(n); p && p !== node; p = parentOf(p)) {
      d++;
    }
    return d;
  };
  const boxes = [];
  const sources = () => [node, ...below].filter(n => chosen.has(n.id));
  const refresh = () => {
    const count = chosen.size;
    summary.textContent = count === 1 && chosen.has(node.id) ? self : (count ? `${count} selected` : 'None');
    onChange(sources());
  };
  const option = (n, label) => {
    const item = el('label', 'side-filter-item');
    item.style.paddingLeft = `${10 + (n === node ? 0 : (depthOf(n) + 1) * 14)}px`;
    const box = el('input');
    box.type = 'checkbox';
    box.checked = chosen.has(n.id);
    box.addEventListener('change', () => {
      if (box.checked) {
        chosen.add(n.id);
      } else {
        chosen.delete(n.id);
      }
      refresh();
    });
    boxes.push({box, id: n.id});
    item.append(box, el('span', '', label));
    return item;
  };
  const bulk = el('div', 'side-filter-bulk');
  const setAll = on => {
    chosen.clear();
    for (const {box, id} of boxes) {
      box.checked = on;
      if (on) {
        chosen.add(id);
      }
    }
    refresh();
  };
  const all = el('button', 'link-button', 'Select All');
  all.type = 'button';
  all.addEventListener('click', () => setAll(true));
  const none = el('button', 'link-button', 'Clear All');
  none.type = 'button';
  none.addEventListener('click', () => setAll(false));
  bulk.append(all, none);
  menu.append(bulk);
  menu.append(option(node, self));
  for (const n of below) {
    menu.append(option(n, n.title));
  }
  toggle.addEventListener('click', e => {
    e.stopPropagation();
    menu.hidden = !menu.hidden;
  });
  menu.addEventListener('click', e => e.stopPropagation());
  document.addEventListener('click', () => {
    menu.hidden = true;
  });
  filter.append(toggle, menu);
  return {wrap: filter, sources, self};
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

function volunteersBox(node, editing, save) {
  const people = shownVolunteers(node, editing).filter(v => v.position !== 'Co-Chair');
  const chairs = coChairs(node);
  const somethingBelow = node.children.some(c => c.status !== 'Hidden' && c.status !== 'Pending');
  const box = el('div', 'vol-box');
  const head = el('div', 'vol-head');
  const title = el('div', 'vol-title');
  const quiet = !node.directSignUp && !chairs.length && !editing;
  const count = chairs.length + people.length;
  if (quiet) {
    box.classList.add('is-quiet');
    if (somethingBelow) {
      title.append(el('div', 'vol-quiet', 'Sign up for something below.'));
    }
  } else {
    title.append(el('h2', 'section section-swoosh', count ? `Who's Involved (${count})` : "Who's Involved"));
  }
  if (editing) {
    const setMax = button(node.spots ? `Max ${node.spots}` : 'Set Max', '', 'button button-secondary button-small vol-max', () => {
      const answer = prompt('How many people can sign up here? Leave blank for no limit.', node.spots ? String(node.spots) : '');
      if (answer === null) {
        return;
      }
      const n = Math.max(0, Math.min(99, Number(answer.trim()) || 0));
      save({spots: n});
    });
    setMax.title = 'Set the most people who can sign up here';
    title.append(setMax);
    const done = el('label', 'side-switch vol-complete');
    done.append(checkbox(Boolean(node.volunteersComplete), on => save({volunteersComplete: on})), el('span', '', 'Volunteers complete'));
    title.append(done);
  }
  const below = descendants(node);
  const listing = el('div', 'vol-listing');
  let filter = null;
  if (below.length) {
    filter = treeFilter(node, below, () => paintListing());
  }
  head.append(title);
  const actions = el('div', 'vol-actions');
  if (filter) {
    actions.append(filter.wrap);
  }
  const mine = mySignUp(node);
  if (node.directSignUp) {
    const me = mine
      ? {label: 'Edit my sign-up', icon: 'edit', onClick: () => openSignUp(node, mine)}
      : (canJoin(node) || node.canEdit ? {label: 'Join', icon: 'join', onClick: () => openSignUp(node, null)} : null);
    const others = node.canEdit || (node.status === 'Open' && !isFull(node));
    if (others) {
      const split = el('div', 'split-button');
      const label = el('button', 'split-label');
      label.type = 'button';
      label.textContent = 'Sign up';
      label.addEventListener('click', () => (me ? me.onClick() : openSignUp(node, null, true)));
      split.append(label);
      if (me) {
        split.append(button(mine ? 'Edit mine' : 'Me', me.icon, 'split-segment', me.onClick));
      }
      split.append(button('Someone else', 'plus', 'split-segment', () => openSignUp(node, null, true)));
      actions.append(split);
    } else if (me) {
      actions.append(button(me.label, me.icon, 'button button-small', me.onClick));
    }
  }
  if (node.runs) {
    const list = el('a', 'button button-small' + (node.emailList ? ' button-secondary' : ''));
    list.href = emailListPath(node);
    list.append(svg('mail'), el('span', '', emailListWords(node)));
    actions.append(list);
  }
  head.append(actions);
  box.append(head);
  const revealed = listRevealed(node, editing);
  const showPeople = node.directSignUp && revealed ? people : [];
  const paintListing = () => {
    listing.replaceChildren();
    const sources = filter ? filter.sources() : [node];
    const own = sources.includes(node);
    const others = sources.filter(n => n !== node);
    if (own) {
      if (chairs.length || showPeople.length) {
        const grid = el('div', 'side-chairs vol-people');
        for (const v of chairs) {
          grid.append(personTile(node, v, editing, false, true));
        }
        const isOption = v => v.position === 'Open to Co-Chair';
        for (const v of showPeople.filter(isOption)) {
          grid.append(personTile(node, v, editing, false, false, true));
        }
        for (const v of showPeople.filter(v => !isOption(v))) {
          grid.append(personTile(node, v, editing, false));
        }
        if (others.length) {
          listing.append(el('div', 'side-group', filter.self));
        }
        listing.append(grid);
      }
      if (!node.directSignUp) {
        if (!quiet && somethingBelow) {
          listing.append(el('div', 'vol-note', 'Sign up for something below.'));
        }
        if (editing && people.length) {
          const n = people.length;
          listing.append(el('div', 'vol-note vol-held',
            `${n} ${n === 1 ? 'person' : 'people'} signed up here before volunteers were turned off. Check Allow volunteers for this itself to show them again.`));
        }
      } else if (!revealed) {
        listing.append(el('div', 'vol-note', 'This list is private; only the organizers see it.'));
      } else if (!people.length && !chairs.length) {
        listing.append(el('div', 'vol-note', 'Nobody yet.'));
      }
    }
    for (const src of others) {
      listing.append(el('div', 'side-group', chainBelow(node, src)));
      const theirs = shownVolunteers(src, editing);
      if (!theirs.length) {
        listing.append(el('div', 'vol-note', 'Nobody yet.'));
        continue;
      }
      const grid = el('div', 'side-chairs vol-people');
      for (const v of theirs) {
        grid.append(personTile(src, v, editing, true));
      }
      listing.append(grid);
    }
    if (!sources.length) {
      listing.append(el('div', 'vol-note', 'Nothing selected.'));
    }
  };
  paintListing();
  box.append(listing);
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
  if (editing) {
    const switches = el('div', 'vol-switches');
    const add = (label, hint, checked, onChange) => {
      const wrap = el('label', 'vol-switch');
      wrap.append(checkbox(checked, onChange));
      const text = el('span');
      text.append(el('strong', '', label));
      if (hint) {
        text.append(el('small', '', hint));
      }
      wrap.append(text);
      switches.append(wrap);
    };
    add('Allow volunteers for this itself', 'Unchecking this will allow volunteers for subcommittees, but not this itself.',
      node.directSignUp, on => save({directSignUp: on}));
    add('Keep Volunteers Secret', 'Only the organizers see who has signed up.', node.volunteersHidden, on => save({volunteersHidden: on}));
    box.append(switches);
  } else if (node.spots) {
    box.append(el('div', 'vol-note', `${node.taken} of ${node.spots} spots taken.`));
  }
  return box;
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

function resourcesCard(node, editing) {
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
    row.append(thumb(item.imageUrl || node.imageUrl || rootOf(node).imageUrl, item.title, 'side-link-thumb'));
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

function flyerCard(node, editing, save) {
  if (!node.flyerUrl && !editing) {
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
  if (editing) {
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

function childrenSection(node, editing) {
  const root = rootOf(node);
  const wrap = el('div');
  const head = el('div', 'list-head');
  head.append(el('h2', 'section section-swoosh', 'Opportunities & Needs'));
  const actions = el('div', 'row-actions');
  let query = '';
  const list = el('div');
  const unlisted = r => r.status === 'Hidden' || r.status === 'Pending';
  const roles = node.children.filter(r => !unlisted(r) || (node.canEdit && (editing || state.showHidden || r.status === 'Pending')));
  if (!roles.length && (node.parent || !eventCategories(root).length)) {
    if (!node.canEdit) {
      return null;
    }
    const bar = el('div', 'add-row');
    bar.append(button('Add Activity', 'plus', 'button button-secondary button-small', () => openActivity(null, {parent: node, category: ''})));
    return bar;
  }
  const groupHead = (cat, others) => {
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
    if (add) {
      add.title = cat ? `Add to ${cat.title}` : 'Add without a category';
      row.append(add);
    }
    return row;
  };
  let reordering = false;
  const reorder = async (id, before, categoryId) => {
    if (reordering) {
      return;
    }
    reordering = true;
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
      await send('POST', '/api/team/order', {parent: node.id, ids});
      await reload();
    } catch (err) {
      toast(err.message);
    } finally {
      reordering = false;
      list.classList.remove('is-reordering');
    }
  };
  const dropTarget = (panel, categoryId) => {
    if (!editing) {
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
      const moved = roles.find(r => r.id === e.dataTransfer.getData('text/plain'));
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
        reorder(moved.id, before, categoryId);
      }
    });
    return panel;
  };
  const render = () => {
    list.replaceChildren();
    const shown = roles.filter(r => matches(r, query));
    const order = [...eventCategories(root).map(c => c.id), ''];
    const present = new Set(shown.map(r => r.category || ''));
    const others = node.parent ? [...present].some(Boolean) : eventCategories(root).length > 0;
    for (const id of order) {
      if (!present.has(id)) {
        continue;
      }
      list.append(groupHead(id ? category(id) : null, others));
      const panel = dropTarget(el('div', 'panel'), id);
      const group = shown.filter(x => (x.category || '') === id);
      group.forEach((r, i) => {
        const moves = editing ? {
          up: i > 0 ? () => reorder(r.id, group[i - 1].id, id) : null,
          down: i < group.length - 1 ? () => reorder(r.id, group[i + 2] ? group[i + 2].id : '', id) : null,
        } : null;
        const row = childRow(r, editing, moves);
        row.dataset.id = r.id;
        if (editing) {
          row.addEventListener('dragover', e => {
            e.preventDefault();
            row.classList.add('is-dropbefore');
          });
          row.addEventListener('dragleave', () => row.classList.remove('is-dropbefore'));
          row.addEventListener('drop', () => row.classList.remove('is-dropbefore'));
        }
        panel.append(row);
      });
      list.append(panel);
    }
    if (!query && !node.parent) {
      for (const c of eventCategories(root)) {
        if (!present.has(c.id)) {
          list.append(groupHead(c));
          if (editing) {
            const panel = dropTarget(el('div', 'panel is-drop-hint'), c.id);
            panel.append(el('div', 'panel-empty', 'Drag something here'));
            list.append(panel);
          }
        }
      }
    }
    if (!shown.length) {
      if (!eventCategories(root).length || node.parent) {
        list.append(groupHead(null, false));
      }
      const panel = el('div', 'panel');
      panel.append(el('div', 'panel-empty', query ? 'Nothing matches.' : 'Nothing here yet.'));
      list.append(panel);
    }
  };
  if (roles.length >= 10) {
    actions.append(searchBox('Search', q => {
      query = q;
      render();
    }, true));
  }
  head.append(actions);
  wrap.append(head);
  render();
  wrap.append(list);
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

function editorBand(node) {
  const band = el('div', 'editor-band');
  band.append(button('Edit', 'edit', 'button button-secondary button-small', () => openActivity(node)));
  if (node.addedBy) {
    const when = node.added ? ` on ${longDate(node.added)}` : '';
    band.append(el('div', 'footnote', `Proposed by ${node.addedBy}${when}`));
  }
  return band;
}

export function activityPage(node) {
  const root = rootOf(node);
  const parent = parentOf(node);
  const save = changes => saveActivityFields(node, changes);
  setTitle(node.title);
  const editing = node.canEdit && editingPath === node.id;
  const page = el('div', 'detail');

  const top = el('div', 'detail-top');
  const back = link(parent ? activityPath(parent) : '/', 'detail-back');
  back.append(svg('back'), el('span', '', parent ? `Back to ${parent.title}` : 'Back to Opportunities'));
  top.append(back, prevNext(node));
  page.append(top);

  if (editing) {
    page.append(editCrumb(node, parent, root, save));
  }

  const hero = el('div', 'detail-hero');
  hero.append(thumb(node.imageUrl || root.imageUrl, node.title, 'detail-hero-image ' + categoryClass(root.category)));
  const stamp = heroStamp(node);
  if (stamp) {
    hero.append(stamp);
  }
  const heroActions = el('div', 'hero-actions');
  if (node.canEdit) {
    const toggle = heroButton(editing ? 'join' : 'edit', editing ? 'Done editing' : 'Edit this page', () => {
      editingPath = editing ? null : node.id;
      document.dispatchEvent(new CustomEvent('hca:refresh'));
    });
    if (editing) {
      toggle.classList.add('is-editing');
    }
    heroActions.append(toggle);
  }
  heroActions.append(shareButton(node));
  if (node.imageUrl) {
    heroActions.append(heroButton('expand', 'View full size', () => openPhotoLightbox(node.imageUrl)));
  }
  hero.append(heroActions);
  if (editing) {
    let statuses = node.status === 'Hidden' ? ['Hidden', 'Open', 'Done'] : ['Open', 'Done'];
    if (isAdmin()) {
      statuses = ['Pending', 'Open', 'Done', 'Hidden'];
    } else if (node.status === 'Pending') {
      statuses = parent && parent.canEdit ? ['Pending', 'Open', 'Done'] : [];
    }
    if (statuses.length) {
      const status = el('label', 'hero-status');
      status.append(el('span', '', 'Status'), statusSelect(node.status, statuses, value => save({status: value})));
      hero.append(status);
    }
    hero.append(heroImageBar(node, save));
  }
  page.append(hero);

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const marks = el('div', 'detail-marks');
  if (!editing) {
    if (parent) {
      marks.append(link(activityPath(parent), 'card-chip is-inline is-link ' + categoryClass(root.category), parent.title));
    } else if (category(node.category)) {
      marks.append(el('span', 'card-chip is-inline ' + categoryClass(node.category), category(node.category).title));
    }
  }
  if (node.coLeaderNeeded) {
    marks.append(el('span', 'need', 'Co-chair needed!'));
  }
  if (node.status !== 'Open') {
    marks.append(badge(node.status === 'Pending' ? 'Needs approval' : node.status, node.status.toLowerCase()));
    if (node.status === 'Pending' && isSystemAdmin() && !editing) {
      marks.append(...approvalButtons(node));
    }
  } else if (isFull(node)) {
    marks.append(completeBadge());
  }
  if (editing && !parent) {
    marks.append(button('Edit Categories', 'list', 'button button-secondary button-small', () => openCategoryManager(node)));
  }
  if (marks.children.length) {
    main.append(marks);
  }
  const heading = el('div', 'detail-heading');
  const title = el('h1', 'detail-title', node.title);
  heading.append(title);
  if (editing) {
    heading.append(editable(title, 'Edit the title',
      () => {
        const input = textInput(node.title, {maxLength: 120});
        return {input, value: () => input.value.trim()};
      },
      value => save({title: value})));
  }
  main.append(heading);
  const blurb = el('div', 'detail-blurb');
  if (node.description) {
    for (const para of node.description.split(/\n\s*\n/).filter(t => t.trim())) {
      blurb.append(el('p', 'detail-text', para.trim()));
    }
  } else if (editing) {
    blurb.append(el('p', 'detail-text is-empty', 'No description yet.'));
  }
  if (blurb.children.length) {
    main.append(blurb);
    if (editing) {
      main.append(editable(blurb, 'Edit the description',
        () => {
          const input = textAreaInput(node.description || '', 7);
          return {input, hint: 'Leave a blank line between paragraphs.', value: () => input.value};
        },
        value => save({description: value})));
    }
  }

  const highlight = highlightCard(node);
  const highlightHint = 'Clear the headline and body to take the highlight off.';
  if (highlight) {
    main.append(highlight);
    if (editing) {
      main.append(editable(highlight, 'Edit the highlight',
        () => {
          const inputs = highlightInputs(node.highlight);
          return {input: inputs.wrap, hint: highlightHint, value: inputs.value};
        },
        value => save({highlight: value})));
    }
  } else if (editing) {
    const add = button('Add Highlight Section', 'plus', 'button button-secondary button-small', () => {
      const inputs = highlightInputs(null);
      fieldEditor(slot, add, {input: inputs.wrap, hint: highlightHint, value: inputs.value, submit: value => save({highlight: value})});
    });
    const slot = el('div', 'highlight-add');
    slot.append(add);
    main.append(slot);
  }
  const resources = resourcesCard(node, editing);
  if (resources) {
    resources.classList.add('resources-main');
    main.append(resources);
  }
  const facts = factsCard(node, editing, save);
  if (facts && phone.matches) {
    facts.classList.add('facts-inline');
    main.append(facts);
  }

  const under = node.children.filter(c => c.status !== 'Hidden' && c.status !== 'Pending');
  const familyRoles = familyBox(node);
  if (familyRoles) {
    main.append(familyRoles);
  }
  main.append(volunteersBox(node, editing, save));
  if (!parent || under.length || node.canEdit) {
    const things = childrenSection(node, editing);
    if (things) {
      main.append(things);
    }
  }
  if (!parent && under.length) {
    main.append(el('div', 'footnote', 'To leave a committee, open it and use Edit my sign-up.'));
  }
  if (node.canEdit) {
    main.append(editorBand(node));
  }

  const side = el('aside', 'detail-side');
  for (const card of [emailListCard(node), phone.matches ? null : facts, flyerCard(node, editing, save), helpCard(node), priorityCard(node, save)]) {
    if (card) {
      side.append(card);
    }
  }
  cols.append(main, side);
  page.append(cols);
  return page;
}
