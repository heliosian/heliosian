import {state, mine, matches} from '../state.js';
import {el, tabs, searchBox} from '../dom.js';
import {setTitle} from '../chrome.js';
import {staffRow, emptyPanel, dateSummary} from '../cards.js';

let tab = 'tasks';
let query = '';

const items = [
  {key: 'tasks', label: 'My Tasks', stages: ['Awaiting Outreach', 'Awaiting Response'], empty: "There's work to be done!"},
  {key: 'wait', label: 'Wait', stages: ['Wait'], empty: 'This is a waiting area. These staff do not yet need outreach.'},
  {key: 'done', label: 'All Done', stages: ['Awaiting Newsletter', 'Complete'], empty: "Thank you so much! You're all done!"},
];

function list() {
  const item = items.find(i => i.key === tab);
  const rows = state.model.staff.filter(sv => mine(sv) && item.stages.includes(sv.stage) && matches(sv, query));
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
  const body = el('div');
  const render = () => {
    body.replaceChildren(tabs(items, tab, key => {
      tab = key;
      render();
    }));
    const head = el('div', 'list-head');
    head.append(el('h1', '', items.find(i => i.key === tab).label));
    head.append(searchBox('Search', q => {
      query = q;
      body.querySelector('.list').replaceChildren(list());
    }));
    const wrap = el('div', 'list');
    wrap.append(list());
    body.append(head, wrap);
  };
  render();
  page.append(body);
  return page;
}
