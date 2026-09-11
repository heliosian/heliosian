import {state, byDepartment, matches} from '../state.js';
import {el, button, tabs, searchBox} from '../dom.js';
import {setTitle} from '../chrome.js';
import {staffRow, emptyPanel} from '../cards.js';
import {openBirthday, openParticipation} from '../edit.js';

let tab = 'missing';
let query = '';

function list() {
  const root = el('div');
  const source = tab === 'missing' ? state.model.missing : state.model.skipped;
  const rows = source.filter(sv => matches(sv, query));
  if (!rows.length) {
    root.append(emptyPanel(query ? 'Nothing matches.' : (tab === 'missing' ? 'Every staff member has a birthday on file.' : 'Nobody has opted out.')));
    return root;
  }
  for (const group of byDepartment(rows)) {
    root.append(el('div', 'section-title', group.name));
    const panel = el('div', 'panel');
    for (const sv of group.items) {
      const lines = [];
      if (tab === 'skipped' && sv.levelNote) {
        lines.push(sv.levelNote);
      }
      if (tab === 'skipped' && sv.birthdayThisYear) {
        lines.push(`Birthday ${sv.birthdayThisYear}`);
      }
      const items = tab === 'missing'
        ? [{icon: 'plus', label: 'Add birthday', onClick: () => openBirthday(sv)}, {icon: 'skipped', label: 'Opt out', onClick: () => openParticipation(sv)}]
        : [{icon: 'edit', label: 'Edit preference', onClick: () => openParticipation(sv)}, {icon: 'plus', label: sv.birthday ? 'Edit birthday' : 'Add birthday', onClick: () => openBirthday(sv)}];
      panel.append(staffRow(sv, {noActions: true, lines, menu: items}));
    }
    root.append(panel);
  }
  return root;
}

export function skippedPage() {
  setTitle('Skipped');
  const page = el('div', 'list-page');
  const body = el('div');
  const render = () => {
    body.replaceChildren(tabs([{key: 'missing', label: 'Missing Birthday'}, {key: 'skipped', label: 'Skipped Birthdays'}], tab, key => {
      tab = key;
      render();
    }));
    const head = el('div', 'list-head');
    head.append(el('h1', '', tab === 'missing' ? 'Missing Birthdays' : 'Opted Out'));
    const actions = el('div', 'row-actions');
    actions.append(searchBox('Search', q => {
      query = q;
      body.querySelector('.list').replaceChildren(list());
    }));
    actions.append(button('Add', 'plus', 'button', () => (tab === 'missing' ? openBirthday({}) : openParticipation({}))));
    head.append(actions);
    body.append(head, el('div', 'section-note', tab === 'missing' ? 'Staff the directory lists who have no birthday on file. Add one, or record that they would rather not take part.' : 'Staff who asked to be left out entirely. Someone who only wants to stay out of the newsletter is still in the process, marked on their page.'));
    const wrap = el('div', 'list');
    wrap.append(list());
    body.append(wrap);
  };
  render();
  page.append(body);
  return page;
}
