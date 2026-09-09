import {byEmail, staleYears} from './state.js';
import {el, svg, withFrom, firstName, infoBanner} from './dom.js';
import {familyOf} from './families.js';
import {personSlug} from './people.js';
import {submitMedia} from './edit.js';

function agedPast(present, updated, years) {
  if (!present) {
    return false;
  }
  const when = Date.parse(updated);
  return Number.isNaN(when) || Date.now() - when > years * 365.25 * 24 * 60 * 60 * 1000;
}

export function monthYear(dateStr) {
  const when = Date.parse(dateStr);
  if (Number.isNaN(when)) {
    return '';
  }
  return new Date(when).toLocaleDateString('en-US', {month: 'long', year: 'numeric'});
}

export function photoNeedsUpdate(p) {
  return p.isStudent && (!p.photoUrl || agedPast(p.photoUrl, p.photoUpdated, staleYears.photo));
}

export function factsNeedUpdate(p) {
  return p.isStudent && (!p.facts || agedPast(p.facts, p.factsUpdated, staleYears.facts));
}

// Unlike a person's own photo, a missing family photo needs updating just as much as
// a stale one.
export function familyPhotoNeedsUpdate(family) {
  return !family.photoUrl || agedPast(family.photoUrl, family.photoUpdated, staleYears.familyPhoto);
}

export function staleItems() {
  const me = byEmail[document.body.dataset.userEmail];
  if (!me) {
    return [];
  }
  const family = familyOf(me);
  const items = [];
  for (const p of familyNavPeople()) {
    const whose = p.email === me.email ? 'your' : `${p.fullName}'s`;
    const shortWhose = p.email === me.email ? 'your' : `${firstName(p.fullName)}'s`;
    if (photoNeedsUpdate(p)) {
      items.push({type: 'photo', target: 'person', key: p.email, text: `Update ${whose} photo for new year`, label: `${shortWhose} photo`, person: p});
    }
    if (factsNeedUpdate(p)) {
      items.push({type: 'facts', target: 'person', key: p.email, text: `Update ${whose} facts for new year`, label: `${shortWhose} facts`, person: p});
    }
  }
  if (family && familyPhotoNeedsUpdate(family)) {
    items.push({type: 'photo', target: 'family', key: family.key, text: 'Update your family photo for new year', label: 'your family photo'});
  }
  return items;
}

// The banner's description names the actual items ("Sam's photo, Ella's photo…")
// rather than just a count, so it's useful at a glance without opening My Family.
export function familyInfoBanner(items) {
  const count = items.length;
  const title = `${count} Update${count === 1 ? '' : 's'} Needed`;
  const desc = items.map(i => i.label).join(', ');
  return infoBanner('alert', 'alert', title, desc, 'Update Family Info', '/my-family', false);
}

function todoPhotoRow(item) {
  const row = el('label', 'todo-row');
  row.append(el('div', 'todo-mark'));
  row.append(el('div', 'todo-text', item.text));
  const status = el('div', 'todo-status');
  const input = el('input');
  input.type = 'file';
  input.accept = 'image/*';
  input.hidden = true;
  input.addEventListener('change', () => {
    if (input.files.length) {
      row.classList.add('todo-row-busy');
      submitMedia(item.target, item.key, 'photo', input.files[0], input.files[0].name, status);
    }
  });
  row.append(input, status);
  const action = el('div', 'todo-chevron');
  action.append(svg('camera'));
  row.append(action);
  return row;
}

function todoFactsRow(item) {
  const row = el('a', 'todo-row');
  row.href = withFrom(`/people/${encodeURIComponent(personSlug(item.key))}?edit=1&focus=facts`);
  row.append(el('div', 'todo-mark'));
  row.append(el('div', 'todo-text', item.text));
  const chev = el('div', 'todo-chevron');
  chev.append(svg('chevron-right'));
  row.append(chev);
  return row;
}

export function todoChecklist(items) {
  const card = el('div', 'todo-card');
  card.append(el('div', 'todo-card-title', `${items.length} thing${items.length === 1 ? '' : 's'} to update for the new year`));
  for (const item of items) {
    card.append(item.type === 'photo' ? todoPhotoRow(item) : todoFactsRow(item));
  }
  return card;
}

export function familyNavPeople() {
  const me = byEmail[document.body.dataset.userEmail];
  if (!me || (me.isStaff && !me.isParent)) {
    return [];
  }
  const family = familyOf(me);
  const emails = [me.email, ...((family && family.adultEmails) || []), ...((family && family.kidEmails) || [])];
  const seen = new Set();
  const people = [];
  for (const email of emails) {
    if (seen.has(email)) {
      continue;
    }
    seen.add(email);
    const p = byEmail[email];
    if (p) {
      people.push(p);
    }
  }
  return people;
}

export function personTodoCount(p) {
  return (photoNeedsUpdate(p) ? 1 : 0) + (factsNeedUpdate(p) ? 1 : 0);
}
