import {state, byDepartment, matches} from '../state.js';
import {el, button, tabs, pageHead} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
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
  const head = el('div');
  const body = el('div');
  const render = () => {
    head.replaceChildren(pageHead('Skipped', [button('Add', 'plus', 'button', () => (tab === 'missing' ? openBirthday({}) : openParticipation({})))]));
    body.replaceChildren(tabs([{key: 'missing', label: 'Missing Birthday', count: state.model.missing.length}, {key: 'skipped', label: 'Opted Out', count: state.model.skipped.length}], tab, key => {
      tab = key;
      render();
    }));
    body.append(el('div', 'section-note', tab === 'missing' ? 'Staff the directory lists who have no birthday on file. Add one, or record that they would rather not take part.' : 'Staff who asked to be left out entirely. Someone who only wants to stay out of the newsletter is still in the process, marked on their page.'));
    const wrap = el('div', 'list');
    wrap.append(list());
    body.append(wrap);
  };
  query = '';
  setSearch('Search staff…', q => {
    query = q;
    body.querySelector('.list').replaceChildren(list());
  });
  render();
  page.append(head, body);
  return page;
}
