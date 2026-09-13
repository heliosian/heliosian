import {state, mine, matches} from '../state.js';
import {el, tabs, pageHead} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {staffRow, emptyPanel, dateSummary} from '../cards.js';

let tab = 'tasks';
let query = '';

const items = [
  {key: 'tasks', label: 'My Tasks', stages: ['Awaiting Outreach', 'Awaiting Response'], empty: "There's work to be done!"},
  {key: 'wait', label: 'Wait', stages: ['Wait'], empty: 'This is a waiting area. These staff do not yet need outreach.'},
  {key: 'done', label: 'All Done', stages: ['Awaiting Newsletter', 'Complete'], empty: "Thank you so much! You're all done!"},
];

function inTab(sv, item) {
  return mine(sv) && item.stages.includes(sv.stage);
}

function list() {
  const item = items.find(i => i.key === tab);
  const rows = state.model.staff.filter(sv => inTab(sv, item) && matches(sv, query));
  if (!rows.length) {
    return emptyPanel(query ? 'Nothing matches.' : item.empty);
  }
  const panel = el('div', 'panel');
  for (const sv of rows) {
    panel.append(staffRow(sv, {stage: true, lines: [dateSummary(sv)]}));
  }
  return panel;
}

export function jobsPage() {
  setTitle('My Jobs');
  const page = el('div', 'list-page');
  page.append(pageHead('My Jobs'));
  const body = el('div');
  const render = () => {
    const counted = items.map(item => ({...item, count: state.model.staff.filter(sv => inTab(sv, item)).length}));
    body.replaceChildren(tabs(counted, tab, key => {
      tab = key;
      render();
    }));
    const wrap = el('div', 'list');
    wrap.append(list());
    body.append(wrap);
  };
  query = '';
  setSearch('Search my jobs…', q => {
    query = q;
    body.querySelector('.list').replaceChildren(list());
  });
  render();
  page.append(body);
  return page;
}
