import {staleYears, viewer, isStudent, isStaff, isParent, familyOf, adultsOf, kidsOf, photosOf, canEditFamily} from './state.js';
import {withFrom, firstName, infoBanner} from './dom.js';
import {el, svg} from '/elements.js';
import {personPath} from './people.js';
import {uploadPhoto} from './edit.js';

function agedPast(updated, years) {
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
  return isStudent(p) && (!photosOf(p.id).length || agedPast(p.photo_updated, staleYears().photo));
}

export function factsNeedUpdate(p) {
  return isStudent(p) && (!p.facts || agedPast(p.facts_updated, staleYears().facts));
}

export function familyPhotoNeedsUpdate(family) {
  return !photosOf(family.id).length;
}

export function staleItems() {
  const me = viewer();
  if (!me) {
    return [];
  }
  const family = familyOf(me);
  const items = [];
  for (const p of familyNavPeople()) {
    const whose = p.id === me.id ? 'your' : `${p.name_show}'s`;
    const shortWhose = p.id === me.id ? 'your' : `${firstName(p.name_show)}'s`;
    if (photoNeedsUpdate(p)) {
      items.push({type: 'photo', target: {person: p.id}, text: `Update ${whose} photo for new year`, label: `${shortWhose} photo`, person: p});
    }
    if (factsNeedUpdate(p)) {
      items.push({type: 'facts', target: {person: p.id}, text: `Update ${whose} facts for new year`, label: `${shortWhose} facts`, person: p});
    }
  }
  if (family && canEditFamily(family) && familyPhotoNeedsUpdate(family)) {
    items.push({type: 'photo', target: {group: family.id}, family: true, text: 'Update your family photo for new year', label: 'your family photo'});
  }
  return items;
}

export function familyInfoBanner(items) {
  const count = items.length;
  const title = `${count} Update${count === 1 ? '' : 's'} Needed`;
  const desc = items.map(i => i.label).join(', ');
  return infoBanner('alert', 'warn', title, desc, 'Update Family Info', '/my-family', false);
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
      uploadPhoto(item.target, input.files[0], status);
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
  row.href = withFrom(`${personPath(item.person)}?edit=1&focus=facts`);
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
  const me = viewer();
  if (!me || (isStaff(me) && !isParent(me))) {
    return [];
  }
  const family = familyOf(me);
  const people = [me, ...(family ? adultsOf(family) : []), ...(family ? kidsOf(family) : [])];
  return [...new Map(people.map(p => [p.id, p])).values()];
}

export function personTodoCount(p) {
  return (photoNeedsUpdate(p) ? 1 : 0) + (factsNeedUpdate(p) ? 1 : 0);
}
