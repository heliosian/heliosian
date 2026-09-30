import {el, svg} from '/elements.js';
import {face, gradeBadge, personRow} from '/personrow.js';
import {appOrigin} from '/appswitch.js';
import {openModal} from '/modal.js';
import {doneButton} from '/personcard.js';

const saidWords = {yes: 'Yes', maybe: 'Maybe', no: 'No', pending: 'Pending'};

const saidIcons = {yes: 'check', maybe: 'clock', no: 'close', pending: 'mail'};

export function familiesOf(rows) {
  const families = new Map();
  for (const r of rows) {
    const key = r.household || r.key || r.email;
    if (!families.has(key)) {
      families.set(key, []);
    }
    families.get(key).push(r);
  }
  const named = f => (f.find(r => r.familyName) || f[0]).familyName || f[0].name || '';
  return [...families.values()].sort((a, b) => named(a).localeCompare(named(b)));
}

export function familySwitch(byFamily, onChange) {
  const wrap = el('div', 'family-switch');
  const choices = [['People', false], ['Families', true]].map(([label, family]) => {
    const b = el('button', 'family-switch-choice' + (family === byFamily ? ' is-on' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      for (const c of choices) {
        c.classList.toggle('is-on', c === b);
      }
      onChange(family);
    });
    return b;
  });
  wrap.append(...choices);
  return wrap;
}

export function familySaid(answer, change) {
  const said = el(change ? 'button' : 'span', 'family-said is-' + (answer || 'none'));
  said.append(svg(saidIcons[answer] || 'info'), el('span', '', saidWords[answer] || 'No response'));
  if (change) {
    said.type = 'button';
    said.title = 'Change the answer';
    said.append(svg('chevron-down'));
  }
  return said;
}

function openFamilyCard(named, counts, members, gradeColors) {
  const name = named.familyName || named.name || named.email;
  const head = el('div', 'who-head');
  const photo = face({name, photoUrl: named.familyPhoto || named.photoUrl}, 'who-face');
  const names = el('div', 'who-names');
  names.append(el('div', 'who-name', name));
  if (counts.length) {
    names.append(el('div', 'who-sub', counts.join(' · ')));
  }
  head.append(photo, names);
  const list = el('div', 'family-card-members');
  for (const m of members) {
    list.append(personRow(m.person, {
      className: 'family-card-member',
      name: m.person.name || m.person.email,
      lines: [m.role],
      onClick: m.onClick,
      gradeColors,
    }));
  }
  const foot = el('div', 'who-foot');
  if (named.familyKey) {
    const who = el('a', 'button');
    who.href = appOrigin('who') + '/families/' + encodeURIComponent(named.familyKey);
    who.target = '_blank';
    who.rel = 'noopener';
    who.append(svg('open'), el('span', '', 'Open on Helios Who?'));
    foot.append(who);
  }
  foot.append(doneButton());
  openModal('', [head, list, foot], {actions: false, wide: 'person'});
}

export function familyRow(family, members, gradeColors) {
  const named = family.find(r => r.familyName) || family[0];
  const count = kind => members.filter(m => m.kind === kind).length;
  const counts = [[count('parent'), 'parent', 'parents'], [count('kid'), 'kid', 'kids'], [count('guest'), 'guest', 'guests']]
    .filter(([n]) => n).map(([n, one, many]) => `${n} ${n === 1 ? one : many}`);
  const grades = [...new Set(members.filter(m => m.kind === 'kid').map(m => m.person.grade).filter(Boolean))];
  const row = el('div', 'family-row');
  const head = el('button', 'family-row-head');
  head.type = 'button';
  head.title = 'About the ' + (named.familyName || named.name || 'family');
  head.addEventListener('click', () => openFamilyCard(named, counts, members, gradeColors));
  const photo = face({name: named.familyName || named.name, photoUrl: named.familyPhoto || named.photoUrl}, 'family-row-photo');
  if (grades.length) {
    const badges = el('div', 'family-row-grades');
    badges.append(...grades.map(g => gradeBadge(g, gradeColors)));
    photo.append(badges);
  }
  const words = el('div', 'family-row-words');
  words.append(el('div', 'family-row-name', named.familyName || named.name || named.email));
  if (counts.length) {
    words.append(el('div', 'family-row-line', counts.join(' · ')));
  }
  head.append(photo, words);
  const list = el('div', 'family-row-members' + (members.some(m => m.after) ? ' has-said' : ''));
  for (const m of members) {
    const member = el('div', 'family-member');
    const who = el(m.onClick ? 'button' : 'div', 'family-member-who');
    if (m.onClick) {
      who.type = 'button';
      who.addEventListener('click', m.onClick);
    }
    const pic = face(m.person, 'family-member-face', gradeColors);
    if (m.corner) {
      pic.append(m.corner);
    }
    const text = el('div', 'family-member-words');
    text.append(el('div', 'family-member-name', m.name));
    if (m.role) {
      text.append(el('div', 'family-member-line', m.role));
    }
    who.append(pic, text);
    member.append(who);
    if (m.after) {
      member.append(m.after);
    }
    list.append(member);
  }
  row.append(head, list);
  return row;
}
