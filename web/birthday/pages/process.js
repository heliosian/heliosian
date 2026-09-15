import {state, stages, stageName, stageClass, isUnassigned, matches, staffPath, tableDate, weekday, team} from '../state.js';
import {el, link, svg, thumb, button, pageHead} from '../dom.js';
import {setTitle, setSearch} from '../chrome.js';
import {emptyPanel} from '../cards.js';
import {assignToMe} from '../edit.js';

// The pipeline, one tile per stage in the order a birthday moves through it,
// with the unassigned ones gathered first so they get picked up.
const pipeline = [
  {key: 'unassigned', icon: 'users', tint: 'grey', note: 'These staff members are not assigned to anyone yet. Pick some up for yourself.'},
  {key: 'Wait', icon: 'calendar', tint: 'teal', note: 'These staff members are assigned, but their outreach date hasn\'t arrived yet.'},
  {key: 'Awaiting Outreach', icon: 'send', tint: 'yellow', note: 'These staff members are due for outreach. If they are assigned to you, please contact them soon.'},
  {key: 'Awaiting Response', icon: 'chat', tint: 'orange', note: 'Waiting to hear back. If they do not reply before the newsletter deadline, the donation goes to the default charity.'},
  {key: 'Awaiting Newsletter', icon: 'newsletter', tint: 'purple', note: 'Good job, team! Now the comms team carries these into the newsletter.'},
  {key: 'Complete', icon: 'circlecheck', tint: 'green', note: 'Everything is all done.'},
];

const dayFormat = new Intl.DateTimeFormat('en-US', {weekday: 'long', month: 'long', day: 'numeric'});

// tab is the stage in view, '' for every stage at once.
let tab = '';
let query = '';
let filters = {department: '', month: '', assignedTo: '', sort: 'contact'};

function inTab(sv, key) {
  return !key || (key === 'unassigned' ? isUnassigned(sv) : sv.stage === key);
}

function tileName(key) {
  return key === 'unassigned' ? 'Unassigned' : stageName(key);
}

function monthOf(s) {
  return s ? s.slice(5, 7) : '';
}

const sorts = {
  contact: (a, b) => (a.requestBy || '9').localeCompare(b.requestBy || '9'),
  birthday: (a, b) => (a.birthdayThisYear || '9').localeCompare(b.birthdayThisYear || '9'),
  name: (a, b) => a.name.localeCompare(b.name),
};

function rowsFor(key) {
  return state.model.staff
    .filter(sv => inTab(sv, key))
    .filter(sv => matches(sv, query))
    .filter(sv => !filters.department || sv.department === filters.department)
    .filter(sv => !filters.month || monthOf(sv.birthdayThisYear) === filters.month)
    .filter(sv => !filters.assignedTo || sv.assignedTo === filters.assignedTo)
    .sort(sorts[filters.sort]);
}

// tiles is the pipeline: each stage with its count, the one in view ringed.
function tiles(onPick) {
  const strip = el('div', 'pipeline');
  pipeline.forEach((stage, i) => {
    if (i) {
      const arrow = el('div', 'pipeline-arrow');
      arrow.append(svg('chevron'));
      strip.append(arrow);
    }
    const tile = el('button', `pipeline-tile tint-${stage.tint}${stage.key === tab ? ' is-active' : ''}`);
    tile.type = 'button';
    const icon = el('div', 'pipeline-icon');
    icon.append(svg(stage.icon));
    const body = el('div');
    body.append(el('div', 'pipeline-label', tileName(stage.key)), el('div', 'pipeline-count', String(state.model.staff.filter(sv => inTab(sv, stage.key)).length)));
    tile.append(icon, body);
    // A tile picks its stage; the one already picked clears back to all.
    tile.addEventListener('click', () => onPick(stage.key === tab ? '' : stage.key));
    strip.append(tile);
  });
  return strip;
}

// summary is the band under the pipeline: the stage in view, how many are in
// it and what it means, and the next date that matters for them.
function summary(stage, rows) {
  const everything = {key: '', icon: 'jobs', tint: 'teal', note: 'Every birthday this year. Pick a stage above, or filter below - by who holds them, say.'};
  stage = stage || everything;
  const band = el('div', `stage-summary tint-${stage.tint}`);
  const icon = el('div', 'summary-icon');
  icon.append(svg(stage.icon));
  const body = el('div', 'summary-body');
  const title = el('div', 'summary-title');
  title.append(el('strong', '', stage.key ? tileName(stage.key) : 'All stages'), el('span', '', ` · ${rows.length} ${rows.length === 1 ? 'birthday' : 'birthdays'}`));
  body.append(title, el('div', 'summary-note', stage.note));
  band.append(icon, body);
  const byNewsletter = ['Awaiting Response', 'Awaiting Newsletter'].includes(stage.key);
  const dated = rows.filter(sv => byNewsletter ? sv.newsletterDate : sv.requestBy).sort((a, b) => byNewsletter ? a.newsletterDate.localeCompare(b.newsletterDate) : a.requestBy.localeCompare(b.requestBy));
  if (dated.length && stage.key && stage.key !== 'Complete') {
    const next = dated.find(sv => (byNewsletter ? sv.newsletterDate : sv.requestBy) >= state.model.today) || dated[0];
    const date = byNewsletter ? next.newsletterDate : next.requestBy;
    const past = date < state.model.today;
    const side = el('div', 'summary-next');
    const cal = el('div', 'summary-next-icon');
    cal.append(svg(byNewsletter ? 'newsletter' : 'calendar'));
    const text = el('div');
    text.append(el('div', 'summary-next-label', byNewsletter ? 'Next newsletter' : past ? 'Outreach due' : 'Next outreach'), el('div', 'summary-next-date', dayFormat.format(new Date(date + 'T00:00:00'))), el('div', 'summary-next-who', next.name));
    side.append(cal, text);
    band.append(side);
  }
  return band;
}

function selectOf(options, value, onChange) {
  const sel = el('select', 'filter-select');
  for (const o of options) {
    const opt = el('option', '', o.label);
    opt.value = o.value;
    sel.append(opt);
  }
  sel.value = value;
  sel.addEventListener('change', () => onChange(sel.value));
  return sel;
}

// filterBar is the search and the pickers over the table: department, birthday
// month, who it is assigned to, and the order.
function filterBar(rerender) {
  const bar = el('div', 'filter-bar');
  const search = el('div', 'filter-search');
  search.append(svg('search'));
  const input = el('input');
  input.type = 'search';
  input.placeholder = 'Search by name or role…';
  input.value = query;
  input.addEventListener('input', () => {
    query = input.value.trim().toLowerCase();
    rerender(false);
  });
  search.append(input);
  bar.append(search);
  const set = key => value => {
    filters = {...filters, [key]: value};
    rerender(false);
  };
  // Status is the same choice as the tiles above, and clears them too.
  const status = selectOf([{label: 'All Statuses', value: ''}, ...pipeline.map(st => ({label: tileName(st.key), value: st.key}))], tab, key => {
    tab = key;
    rerender(true);
  });
  status.classList.add('filter-status');
  bar.append(status);
  bar.append(selectOf([{label: 'All Roles', value: ''}, ...state.model.departments.map(d => ({label: d, value: d}))], filters.department, set('department')));
  const months = [];
  const start = new Date(state.model.year.start + 'T00:00:00');
  for (let i = 0; i < 12; i++) {
    const d = new Date(start.getFullYear(), start.getMonth() + i, 1);
    months.push({label: d.toLocaleString('en-US', {month: 'long', year: 'numeric'}), value: String(d.getMonth() + 1).padStart(2, '0')});
  }
  bar.append(selectOf([{label: 'All Months', value: ''}, ...months], filters.month, set('month')));
  const assignees = new Map();
  for (const sv of state.model.staff) {
    if (sv.assignedTo) {
      assignees.set(sv.assignedTo, sv.assignedToName || sv.assignedTo);
    }
  }
  for (const p of team()) {
    if (!assignees.has(p.email)) {
      assignees.set(p.email, p.name === 'Me' ? state.model.user.name : p.name);
    }
  }
  bar.append(selectOf([{label: 'Assigned to', value: ''}, ...[...assignees].sort((a, b) => a[1].localeCompare(b[1])).map(([value, label]) => ({label, value}))], filters.assignedTo, set('assignedTo')));
  const sort = el('div', 'filter-sort');
  sort.append(svg('sort'), selectOf([{label: 'Sort by Contact Date', value: 'contact'}, {label: 'Sort by Birthday', value: 'birthday'}, {label: 'Sort by Name', value: 'name'}], filters.sort, set('sort')));
  bar.append(sort);
  return bar;
}

function dateCell(icon, date) {
  const cell = el('div', icon === 'mail' ? 'cell-date contact' : 'cell-date');
  cell.append(svg(icon));
  const text = el('div');
  text.append(el('div', '', date ? tableDate(date) : '—'), el('div', 'cell-sub', date ? `(${weekday(date)})` : ''));
  cell.append(text);
  return cell;
}

function row(sv) {
  const tr = link(staffPath(sv), 'trow');
  tr.dataset.stage = stageClass(sv.stage);
  const who = el('div', 'cell-who');
  who.append(thumb(sv, 'small'), el('div', 'cell-name', sv.name));
  const assignee = el('div', 'cell-assignee');
  if (sv.assignedTo) {
    assignee.append(thumb({name: sv.assignedToName || sv.assignedTo}, 'tiny'), el('span', '', (sv.assignedToName || sv.assignedTo).split(' ')[0]));
  } else {
    assignee.append(el('span', 'cell-sub', 'Unassigned'));
  }
  const status = el('div', 'cell-status');
  if (isUnassigned(sv)) {
    status.append(button('Assign to Me', 'bolt', 'button button-small', () => assignToMe(sv)));
  } else {
    status.append(el('span', 'status-pill ' + stageClass(sv.stage), stageName(sv.stage)));
  }
  status.append(svg('chevron'));
  tr.append(who, el('div', 'cell-role', sv.jobTitle || ''), dateCell('cake', sv.birthdayThisYear), dateCell('mail', sv.requestBy), assignee, status);
  return tr;
}

function table(rows, rerender) {
  const wrap = el('div', 'staff-table');
  const head = el('div', 'trow thead');
  for (const label of ['Staff Member', 'Role', 'Birthday', 'Contact Date', 'Assigned To', 'Status/Actions']) {
    head.append(el('div', '', label));
  }
  wrap.append(head);
  if (!rows.length) {
    wrap.append(el('div', 'table-empty', query || filters.department || filters.month || filters.assignedTo ? 'Nothing matches.' : tab === 'unassigned' ? 'Everyone is assigned.' : tab ? 'Nobody here right now.' : 'No birthdays this year.'));
    return wrap;
  }
  for (const sv of rows) {
    wrap.append(row(sv));
  }
  const foot = el('div', 'table-foot');
  foot.append(el('span', '', `${rows.length} ${rows.length === 1 ? 'birthday' : 'birthdays'}`));
  wrap.append(foot);
  return wrap;
}

export function processPage() {
  setTitle('Process');
  const page = el('div', 'list-page process-page');
  const head = pageHead('Birthday Outreach');
  head.querySelector('.page-head-main').append(el('div', 'page-subtitle', 'Manage staff birthday newsletter requests'));
  page.append(head);
  const body = el('div');
  const render = whole => {
    const stage = pipeline.find(s => s.key === tab);
    const rows = rowsFor(tab);
    if (whole) {
      body.replaceChildren(tiles(key => {
        tab = key;
            render(true);
      }), summary(stage, rows), filterBar(render), table(rows, render));
      return;
    }
    body.querySelector('.stage-summary').replaceWith(summary(stage, rows));
    body.querySelector('.staff-table').replaceWith(table(rows, render));
  };
  query = '';
  setSearch('Search staff…', q => {
    query = q;
    const input = body.querySelector('.filter-search input');
    if (input) {
      input.value = q;
    }
    render(false);
  });
  render(true);
  page.append(body);
  return page;
}
