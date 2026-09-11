import {state, isAdmin, years, allYears, descendants, parentOf, rootOf, category, eventCategories, longDate, parseWhen, coChairs, mySignUp, canJoin, isFull, matches, activityPath, listedIn, sortByStart, shiftedEnd, headingChoices, shownVolunteers, listHidden, listRevealed, canAdd, addLabel, ADDING} from '../state.js';
import {el, link, svg, thumb, avatar, badge, button, searchBox, copyText, whenEditor, toast} from '../dom.js';
import {setTitle} from '../chrome.js';
import {childRow, categoryClass} from '../cards.js';
import {openCropTool, openPhotoLightbox} from '../crop.js';
import {send, reload, openSignUp, openActivity, openLink, saveActivityFields, openPerson, openImageSearch, imageSearchOn, editable, fieldEditor, highlightInputs, textInput, textAreaInput, selectInput, uploadAndSave, openCategoryManager, openVolunteerGrid} from '../edit.js';


// Which activity is in edit mode, by id. Keyed rather than a bare boolean so
// opening a different activity never inherits the last one's edit mode. Mirrors
// Helios Who?'s personEdit: one Edit button reveals every field's pencil, and
// becomes Done.
let editingPath = null;

const weekdayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'short'});
const monthShort = new Intl.DateTimeFormat('en-US', {month: 'short'});
const fullDate = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'});
const clock = new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: '2-digit'});

function timeRange(node) {
  const start = parseWhen(node.start);
  if (!start || !start.hasTime) {
    return '';
  }
  const end = parseWhen(node.end);
  return end && end.hasTime ? `${clock.format(start.date)} – ${clock.format(end.date)}` : clock.format(start.date);
}

// heroStamp is the card floating over the hero image: weekday, date, and the
// hours. An activity with no parsed date shows its timing words instead.
function heroStamp(act) {
  const start = parseWhen(act.start);
  if (act.timing || !start) {
    return act.timing ? el('div', 'hero-stamp hero-stamp-text', act.timing) : null;
  }
  const stamp = el('div', 'hero-stamp');
  stamp.append(el('div', 'hero-stamp-weekday', weekdayFormat.format(start.date).toUpperCase()));
  stamp.append(el('div', 'hero-stamp-date', `${monthShort.format(start.date).toUpperCase()} ${start.date.getDate()}`));
  const hours = timeRange(act);
  if (hours) {
    stamp.append(el('div', 'hero-stamp-time', hours));
  }
  return stamp;
}

// heroImageBar is the strip across the foot of the hero while editing: pick a
// file and it uploads and saves in one go, since an image the reader picked but
// did not save would be a half-finished state with nothing to show for it.
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
  // The image comes from a file or from Google Images; with search set up the
  // button is a small menu of the two, otherwise it is the file picker itself.
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
    item('search', 'Find an image', () => openImageSearch(node.title, picked => save({image: picked})));
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
    // Crop opens the tool over the current picture; what is inside the frame
    // is uploaded as a new image and saved in the picture's place.
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

// prevNext walks the same list, in the same order, that the opportunities grid
// shows for this year, so "next" means what the reader would expect from where
// they came in.
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

// facts is the summary rail beside the description: when it happens, where, and
// who runs it. Anything the sheet leaves blank simply doesn't appear.
function factsCard(node, editing, save) {
  const card = sideCard();
  let any = false;
  const start = parseWhen(node.start);
  const when = [];
  // Timing words stand in for the date and time when both are set.
  if (node.timing) {
    when.push(node.timing);
  } else if (start) {
    when.push(fullDate.format(start.date));
    const hours = timeRange(node);
    if (hours) {
      when.push(hours);
    }
  }
  if (when.length && node.whenFrom) {
    when.push(`Same as ${node.whenFrom.title}`);
  }
  if (when.length || editing) {
    const row = sideRow('calendar', 'Date & Time', when.length ? when : ['Not scheduled']);
    const body = row.querySelector('.side-row-body');
    if (editing) {
      // Start, end and the free-text timing are one thought, so they are one
      // editor rather than three pencils in a row.
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
    if (start) {
      body.append(button('Add to Calendar', 'calendar', 'button button-secondary button-small side-button',
        () => downloadCalendar(node)));
    }
    card.append(row);
    any = true;
  }
  const chairs = coChairs(node);
  // Whether a co-chair is wanted lives here, with the co-chairs themselves: the
  // row shows when there are any, when one is wanted, or while editing (so the
  // switch is reachable even when neither is true yet).
  if (chairs.length || node.coLeaderNeeded || editing) {
    const row = el('div', 'side-row');
    const mark = el('div', 'side-icon');
    mark.append(svg('people'));
    row.append(mark);
    const body = el('div', 'side-row-body');
    body.append(el('div', 'side-title', 'Co-Chairs'));
    if (chairs.length) {
      const list = el('div', 'side-chairs');
      for (const v of chairs) {
        list.append(personTile(node, v, editing, false));
      }
      body.append(list);
    } else {
      body.append(el('div', 'side-line', 'Nobody yet.'));
    }
    // Who has offered to co-chair is public - a secret list still keeps its
    // offers to the editors - and for whoever runs the event each is a way
    // into their sign-up, where Co-Chair is theirs to choose.
    const options = shownVolunteers(node, node.canEdit).filter(v => v.position === 'Open to Co-Chair');
    if (options.length) {
      body.append(el('div', 'side-subtitle', 'Co-Chair Options'));
      if (node.canEdit) {
        body.append(el('div', 'side-line', 'Promote one or more to co-chair.'));
      }
      const list = el('div', 'side-chairs');
      for (const v of options) {
        list.append(personTile(node, v, node.canEdit, false, false, true));
      }
      body.append(list);
    }
    if (editing) {
      const wants = el('label', 'side-switch');
      const box = checkbox(node.coLeaderNeeded, on => save({coLeaderNeeded: on}));
      wants.append(box, el('span', '', 'A co-chair is needed'));
      body.append(wants);
    } else if (node.coLeaderNeeded) {
      // One click offers: the viewer's sign-up becomes (or starts as) open to
      // co-chairing, and the chairs see them under Co-Chair Options. Once
      // they have, the ask gives way to the thanks.
      const mine = mySignUp(node);
      if (mine && mine.position === 'Open to Co-Chair') {
        body.append(el('div', 'side-line side-offered', 'You have offered to co-chair - thank you!'));
      } else {
        body.append(el('div', 'side-line side-need', 'A co-chair is needed - could that be you?'));
      }
      if (!mine || mine.position === 'Volunteer') {
        body.append(button('Offer to Co-Chair', 'people', 'button button-small side-offer', async () => {
          try {
            await send('POST', '/api/events/volunteer', {id: node.id, position: 'Open to Co-Chair', note: mine ? mine.note : ''});
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

// personTile is one avatar and name in the rail. It opens the person's window:
// the contact card, with their sign-up alongside for whoever runs the event.
function personTile(owner, v, editing, star, chair, option) {
  const tile = el('button', 'side-chair' + (editing ? ' is-editable' : '') + (chair ? ' is-chair' : '') + (option ? ' is-option' : ''));
  tile.type = 'button';
  tile.title = owner.canEdit ? `${v.name} and their sign-up` : `About ${v.name}`;
  tile.addEventListener('click', () => openPerson(v, owner));
  const face = avatar(v);
  // A student's grade rides on the corner of their face, short: "5", "K".
  if (v.grade) {
    const grade = el('span', 'grade-badge', v.grade.replace(/^grade\s*/i, ''));
    grade.title = v.grade;
    // Helios Who?'s colour for the grade, darkened as Who? darkens it, so
    // white text stays legible even on a yellow.
    const color = (state.model.gradeColors || {})[v.grade];
    if (color) {
      grade.style.background = `color-mix(in srgb, ${color} 65%, black)`;
    }
    face.append(grade);
  }
  tile.append(face, el('div', 'side-chair-name', v.name + (star && v.position === 'Co-Chair' ? '*' : '')));
  if (chair) {
    tile.append(el('div', 'side-chair-role', 'Chair'));
  } else if (option) {
    tile.append(el('div', 'side-chair-role is-option', 'Chair opt'));
  }
  return tile;
}

// whereIs names a node's place in its event the way organizers say it: the top
// committee's category, then each committee down to the node - "Booths > Poland
// > Performance". The event itself is just "(itself)".
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

// editCrumb is the line above the hero while editing: school year, then the
// parent for a child, then the category - each its own thing to change. Only an
// admin moves something between years; the server refuses anyone else, so the
// pencil is not offered to them. The category menu also offers "Change Parent
// Event…", which turns this event into a child of another and drops its page
// category, since children name their event's categories instead.
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
      // Every activity in this year except the row itself and what sits under it;
      // nested ones are labelled by their path so the list reads as a tree.
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

// treeFilter is the dropdown that picks which of a thing's descendants the
// volunteers list includes: a summary button that drops a list of checkboxes,
// one for the thing itself - named as itself, so the label reads the same for
// an event, a committee or a shift - and one per thing under it, indented by
// depth, with Select All / Clear All. `onChange` runs with the chosen nodes.
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
  // Select All / Clear All set every box and the selection in one go, so a
  // long list can be flipped without walking it.
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

// volunteersBox is the sign-up surface for this thing itself: who is on it, the
// way to join, and - while editing - whether people may join it directly and
// whether the list is private. Sign-ups for the things under it live on their
// own pages; the rail's filter is the cross-tree view.
function volunteersBox(node, editing, save) {
  const people = shownVolunteers(node, editing).filter(v => v.position !== 'Co-Chair');
  const chairs = coChairs(node);
  const box = el('div', 'vol-box');
  const head = el('div', 'vol-head');
  const title = el('div', 'vol-title');
  // The count is everyone on it, chairs included.
  const count = chairs.length + people.length;
  title.append(el('h2', 'section section-swoosh', count ? `Volunteers (${count})` : 'Volunteers'));
  if (editing) {
    // The cap on sign-ups sits by the count it caps: a small button, a prompt.
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
  }
  // With things under it, a filter beside the heading brings their
  // volunteers into the list too, each run under its committee's name.
  const below = descendants(node);
  const listing = el('div', 'vol-listing');
  let filter = null;
  if (below.length) {
    filter = treeFilter(node, below, () => paintListing());
  }
  head.append(title);
  // With volunteers not allowed here there is nothing to join and nothing to
  // edit, so the buttons go; a secret list keeps its buttons but shows nobody
  // to anyone not editing or showing hidden things.
  const actions = el('div', 'vol-actions');
  if (filter) {
    actions.append(filter.wrap);
  }
  const mine = mySignUp(node);
  if (node.directSignUp) {
    // Removing yourself lives inside that editor, as its Remove action.
    const me = mine
      ? {label: 'Edit my sign-up', icon: 'edit', onClick: () => openSignUp(node, mine)}
      : (canJoin(node) || node.canEdit ? {label: 'Join', icon: 'join', onClick: () => openSignUp(node, null)} : null);
    // Anyone may sign up someone else - a partner, a child, a friend who said
    // yes - so while it is open the two ways to sign up are one control:
    // "Sign up: Me | Someone else". Whoever runs it keeps it at all times.
    const others = node.canEdit || (node.status === 'Open' && !isFull(node));
    if (others) {
      const split = el('div', 'split-button');
      split.append(el('span', 'split-label', 'Sign up'));
      if (me) {
        split.append(button(mine ? 'Edit mine' : 'Me', me.icon, 'split-segment', me.onClick));
      }
      split.append(button('Someone else', 'plus', 'split-segment', () => openSignUp(node, null)));
      actions.append(split);
    } else if (me) {
      actions.append(button(me.label, me.icon, 'button button-small', me.onClick));
    }
  }
  head.append(actions);
  box.append(head);
  // The chairs lead the list whatever else it shows - they are public, and
  // they keep the section from standing bare - each marked as a chair; then
  // the volunteers, or the reason there are none to show; then, when the
  // filter brings them in, each chosen committee's volunteers under its name.
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
        // Those open to co-chairing come next, marked for everyone - an offer
        // is public, and seeing one is what nudges the next.
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
        // Volunteers are taken only by the things under it, so say so where
        // the sign-up button would be.
        listing.append(el('div', 'vol-note', 'Sign up for something below.'));
      } else if (!revealed) {
        listing.append(el('div', 'vol-note', 'This list is private; only the organizers see it.'));
      } else if (!people.length && !chairs.length) {
        // The chairs above are enough to say who's involved; only an empty
        // section needs the note.
        listing.append(el('div', 'vol-note', 'Nobody yet.'));
      }
    }
    for (const src of others) {
      listing.append(el('div', 'side-group', src.title));
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
    // The organizers' roster: every sign-up in the tree with where it is and
    // how to reach them, including a student's parents - under the list it sums up.
    const roster = el('div', 'vol-roster');
    roster.append(button('Volunteer Info', 'list', 'button button-secondary button-small roster-open',
      () => openVolunteerGrid(node, [node, ...below], n => whereIs(node, n))));
    box.append(roster);
  }
  if (node.status === 'Open' && isFull(node) && !mine && node.directSignUp) {
    box.append(el('div', 'vol-note vol-full', 'Every spot is taken. Thank you, everyone!'));
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

// resourcesCard is the activity's own links, plus the editor's way to add one.
function resourcesCard(node, editing) {
  // The card earns its place with links, or while editing so one can be added.
  if (!node.links.length && !editing) {
    return null;
  }
  // The resources speak for themselves - no heading over them.
  const card = sideCard();
  // Each resource is a picture, its title and a line about it - the host when
  // nobody wrote one - and opens in a new tab. While editing, a pencil beside
  // the row opens its editor.
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

// highlightCard is the callout on the page: the emoji large at the left, the
// headline, and the body with its **bold** runs made bold - the only markup
// the sheet allows, so a line can stress "one performance" without HTML.
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

// flyerCard is the event's poster in the rail, under the details: the whole
// picture at the rail's width, a click to see it full size, and while editing
// a way to put one up or take it down.
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
    // A flyer is a finished poster: it is uploaded whole, never found in a
    // library or cropped.
    bar.append(upload);
    if (node.flyer) {
      bar.append(button('Remove', 'trash', 'button button-secondary button-small', () => save({flyer: ''})));
    }
    card.append(bar);
  }
  return card;
}

// helpCard points at the people who run the activity. Only shown when there is
// somebody to point at - co-chairs are the ones with a published address here.
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
  mail.append(svg('mail'), el('span', '', 'Contact the Co-Chairs'));
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
    // The friendly address when there is one, however this page was reached.
    const url = location.origin + activityPath(node);
    if (navigator.share) {
      try {
        await navigator.share({title: node.title, url});
        return;
      } catch {
        // The reader dismissed the sheet, or the browser refused it - fall
        // through to the clipboard rather than leaving the button dead.
      }
    }
    copyText(url, 'Link copied');
  });
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
function downloadCalendar(node) {
  const act = rootOf(node);
  const start = parseWhen(node.start);
  const end = parseWhen(node.end);
  const lines = ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//HCA//HCA-Team//EN', 'BEGIN:VEVENT',
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
  lines.push(`URL:${location.origin + activityPath(node)}`, 'END:VEVENT', 'END:VCALENDAR');
  const blob = new Blob([lines.join('\r\n')], {type: 'text/calendar'});
  const a = el('a');
  a.href = URL.createObjectURL(blob);
  a.download = `${node.title}.ics`;
  a.click();
  URL.revokeObjectURL(a.href);
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
  const roles = node.children.filter(r => !unlisted(r) || (node.canEdit && (editing || state.showHidden)));
  // Nothing under it yet, and no categories to lay out: the whole section is
  // one button for whoever runs it, and nothing at all for anyone else.
  if (!roles.length && (node.parent || !eventCategories(root).length)) {
    if (!node.canEdit) {
      return null;
    }
    const bar = el('div', 'add-row');
    bar.append(button('Add Activity', 'plus', 'button button-secondary button-small', () => openActivity(null, {parent: node, category: ''})));
    return bar;
  }
  // Children are grouped by the root event's own categories, in the order the
  // event keeps them, with the uncategorised run last. Each category carries its
  // own add button when it allows adding - so a new thing lands where you were
  // looking - and whoever runs the event can add into any of them regardless.
  // `others` says whether named groups are on the page too: the run without a
  // category is only worth calling Uncategorized when there are.
  const groupHead = (cat, others) => {
    const row = el('div', 'group-row');
    row.append(el('div', 'group-title', cat ? cat.title : (others ? 'Uncategorized' : '')));
    const open = () => openActivity(null, {parent: node, category: cat ? cat.id : ''});
    let add = null;
    // Into a category, its policy; straight under the thing, the thing's own.
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
  // reorder saves the children in a new order: every child of the thing, as
  // they stand, with `id` moved to sit just before `before` - or at the end
  // of its category when there is no before. Groups keep their own runs,
  // since the page shows them apart.
  let reordering = false;
  const reorder = async (id, before, categoryId) => {
    // One at a time: a second click before the first has saved would be
    // computed from an order that is about to change.
    if (reordering) {
      return;
    }
    reordering = true;
    list.classList.add('is-reordering');
    const ids = node.children.map(c => c.id).filter(x => x !== id);
    let at = before ? ids.indexOf(before) : -1;
    if (at < 0) {
      // After the last of its category, or at the very end.
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
      await send('POST', '/api/events/order', {parent: node.id, ids});
      await reload();
    } catch (err) {
      toast(err.message);
    } finally {
      reordering = false;
      list.classList.remove('is-reordering');
    }
  };
  // While editing, every category's panel takes a dropped row: the row's id
  // rides along in the drag, and landing it saves the new category - or,
  // dropped among its own siblings, its new place. The browser insists on
  // preventDefault during dragover for a drop to happen.
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
      // Onto one of its siblings: take that row's place; onto the panel's
      // empty space: go last.
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
    // Named groups on this page: the ones with things in them, plus on the
    // event itself every category, since those all get a heading below.
    const others = node.parent ? [...present].some(Boolean) : eventCategories(root).length > 0;
    for (const id of order) {
      if (!present.has(id)) {
        continue;
      }
      list.append(groupHead(id ? category(id) : null, others));
      const panel = dropTarget(el('div', 'panel'), id);
      const group = shown.filter(x => (x.category || '') === id);
      group.forEach((r, i) => {
        // Up swaps with the row above (moving before it); down moves past the
        // row below (before the one after that, or to the end).
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
    // On the event itself a category with nothing in it yet still gets its
    // heading and add button, otherwise there would be no way to put the first
    // thing into it - and, while editing, an empty panel to drop something
    // into. Under a committee only the categories its own things use appear:
    // the event's full list belongs to the event's page.
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
  // A search box only once there is enough to search through.
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

// editorBand is the quiet tail of the page for whoever runs the thing: one
// small button into the full form, and who proposed it. The pencil on the hero
// is the editing entrance; this is the fallback for everything at once.
function editorBand(node) {
  const band = el('div', 'editor-band');
  band.append(button('Edit', 'edit', 'button button-secondary button-small', () => openActivity(node)));
  if (node.addedBy) {
    const when = node.added ? ` on ${longDate(node.added)}` : '';
    band.append(el('div', 'footnote', `Proposed by ${node.addedBy}${when}`));
  }
  return band;
}

// activityPage is every node's page: a headline event and a single shift on
// its sign-up sheet are the same kind of thing at a different depth, so they
// get the same page. What varies follows from having a parent or not.
export function activityPage(node) {
  const root = rootOf(node);
  const parent = parentOf(node);
  const save = changes => saveActivityFields(node, changes);
  setTitle(node.title);
  // Keyed by id, not address: giving the thing a friendly address while
  // editing must not drop it out of edit mode.
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
    // The hero crops a picture to its band; this shows the whole of it.
    heroActions.append(heroButton('expand', 'View full size', () => openPhotoLightbox(node.imageUrl)));
  }
  hero.append(heroActions);
  if (editing) {
    // Status sits right under the pencil that revealed it: a co-chair may only
    // open or finish a thing; an admin can also park it as pending or hidden.
    const statuses = isAdmin() || parent ? ['Pending', 'Open', 'Done', 'Hidden'] : ['Open', 'Done'];
    const status = el('label', 'hero-status');
    status.append(el('span', '', 'Status'), statusSelect(node.status, statuses, value => save({status: value})));
    hero.append(status);
    hero.append(heroImageBar(node, save));
  }
  page.append(hero);

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const marks = el('div', 'detail-marks');
  if (!editing) {
    if (parent) {
      // A child's chip is the thing it sits under, in its root's colour - the way
      // back up, and the reminder of what this is part of.
      marks.append(link(activityPath(parent), 'card-chip is-inline is-link ' + categoryClass(root.category), parent.title));
    } else if (category(node.category)) {
      marks.append(el('span', 'card-chip is-inline ' + categoryClass(node.category), category(node.category).title));
    }
  }
  if (node.coLeaderNeeded) {
    marks.append(el('span', 'need', 'Co-leader needed!'));
  }
  if (node.status !== 'Open') {
    marks.append(badge(node.status === 'Pending' ? 'Needs approval' : node.status, node.status.toLowerCase()));
  } else if (isFull(node)) {
    marks.append(badge('Full', 'full'));
  }
  if (editing && !parent) {
    // An event's own categories are what its committees, booths and shifts are
    // grouped under; whoever runs the event shapes them from here.
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
    // The sheet holds one text cell, so blank lines are the only paragraph
    // marks there are.
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

  // The highlight is the one thing organizers most want read, so it follows
  // the write-up as a warm callout: emoji, headline, a few lines. In edit
  // mode it takes a pencil, or - with none yet - a quiet button to add one.
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
  // Resources read as part of the write-up, so they sit under it rather than in
  // the rail with the facts.
  const resources = resourcesCard(node, editing);
  if (resources) {
    resources.classList.add('resources-main');
    main.append(resources);
  }

  // Volunteers live in the rail now, with a filter over the tree. What sits
  // under this thing is one list; its hidden and pending rows join it only for
  // an editor who is editing or has Show Hidden Things on, muted.
  const under = node.children.filter(c => c.status !== 'Hidden' && c.status !== 'Pending');
  // The volunteers section always shows; what it holds follows the switches
  // (see volunteersBox): no buttons when volunteers are not allowed here, and
  // nobody listed when the list is secret, until editing or Show Hidden Things.
  main.append(volunteersBox(node, editing, save));
  if (!parent || under.length || node.canEdit) {
    const things = childrenSection(node, editing);
    if (things) {
      main.append(things);
    }
  }
  if (!parent) {
    main.append(el('div', 'footnote', 'To leave a committee, open it and use Edit my sign-up.'));
  }
  if (node.canEdit) {
    main.append(editorBand(node));
  }

  const side = el('aside', 'detail-side');
  for (const card of [factsCard(node, editing, save), flyerCard(node, editing, save), helpCard(node)]) {
    if (card) {
      side.append(card);
    }
  }
  cols.append(main, side);
  page.append(cols);
  return page;
}
