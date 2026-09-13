import {state, stages, stageLabel, isUnassigned, matches} from '../state.js';
import {el, tabs, pageHead} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {staffRow, emptyPanel} from '../cards.js';

let tab = 'unassigned';
let query = '';

const notes = {
  'unassigned': 'Please select some staff to assign to yourself.',
  'Wait': 'This is a waiting area. These staff do not yet need outreach.',
  'Awaiting Outreach': 'These staff are due for outreach. If they are assigned to you, please contact them ASAP.',
  'Awaiting Response': 'Awaiting a response from these staff. If they do not respond before the newsletter deadline, we will use the default charity.',
  'Awaiting Newsletter': "Good job, team! Now we're just waiting for the communications team to include these in the newsletter.",
  'Complete': 'Everything is all done.',
};

const items = [{key: 'unassigned', label: '1. Unassigned'}, ...stages.map(s => ({key: s, label: stageLabel(s)}))];

function group(title, rows) {
  const wrap = el('div');
  if (title) {
    wrap.append(el('div', 'section-title', title));
  }
  const panel = el('div', 'panel');
  for (const sv of rows) {
    panel.append(staffRow(sv, {stage: tab === 'unassigned'}));
  }
  wrap.append(panel);
  return wrap;
}

function inTab(sv, key) {
  return key === 'unassigned' ? isUnassigned(sv) : sv.stage === key;
}

function list() {
  const root = el('div');
  if (tab === 'unassigned') {
    const rows = state.model.staff.filter(sv => inTab(sv, tab) && matches(sv, query));
    let any = false;
    for (const stage of ['Awaiting Outreach', 'Awaiting Response', 'Awaiting Newsletter', 'Wait']) {
      const inStage = rows.filter(sv => sv.stage === stage);
      if (!inStage.length) {
        continue;
      }
      any = true;
      root.append(group(stage === 'Awaiting Outreach' ? 'Awaiting Outreach (ASAP)' : stage, inStage));
    }
    if (!any) {
      root.append(emptyPanel(query ? 'Nothing matches.' : 'Everyone is assigned.'));
    }
    return root;
  }
  const rows = state.model.staff.filter(sv => inTab(sv, tab) && matches(sv, query));
  if (!rows.length) {
    root.append(emptyPanel(query ? 'Nothing matches.' : 'Nobody here right now.'));
    return root;
  }
  root.append(group('', rows));
  return root;
}

export function processPage() {
  setTitle('Process');
  const page = el('div', 'list-page');
  page.append(pageHead('Process'));
  const body = el('div');
  const render = () => {
    const counted = items.map(item => ({...item, count: state.model.staff.filter(sv => inTab(sv, item.key)).length}));
    body.replaceChildren(tabs(counted, tab, key => {
      tab = key;
      render();
    }));
    body.append(el('div', 'section-note', notes[tab]));
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
  page.append(body);
  return page;
}
