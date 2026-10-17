import {state, isAdmin} from './state.js';
import {el} from '/elements.js';
import {adminPage as buildAdminPage, adminsCard, groupMembers} from '/admin.js';
import {queries} from '/rail.js';
import {api} from '/api.js';
import {render} from '/router.js';

const feedback = {rows: [], people: {}, apps: {}, filter: 'new', open: new URLSearchParams(location.search).get('report') || ''};

const statusWords = {new: 'New', filed: 'Filed', dismissed: 'Dismissed'};

function nameOf(id) {
  return (feedback.people[id] || {}).name_show || 'someone';
}

function describe(report) {
  const parts = [feedback.apps[report.app] || report.app, (report.added || '').slice(0, 16)].filter(Boolean);
  if (report.status === 'filed') {
    parts.push(`filed by ${nameOf(report.handled_by)}`);
  } else if (report.status === 'dismissed') {
    parts.push(`dismissed by ${nameOf(report.handled_by)}`);
  }
  return parts.join(' · ');
}

function issueLink(report, className, text) {
  const link = el('a', className, text);
  link.href = report.issue;
  link.target = '_blank';
  link.rel = 'noopener';
  return link;
}

function field(tag, label, value, attrs = {}) {
  const wrap = el('label', '', label);
  const input = el(tag);
  Object.assign(input, attrs);
  input.value = value;
  wrap.append(input);
  return {wrap, input};
}

async function loadFeedback() {
  const a = await queries({
    reports: '(from REPORT (order added desc) (include added_by handled_by) (columns app kind summary details url page browser viewport screen language time_zone errors screenshot status issue handled handled_by added_by added))',
    reportApps: '(from APP (columns key name))',
  });
  feedback.rows = a.reports.result.map(id => a.reports.resources.REPORT[id]);
  feedback.people = a.reports.resources.PERSON || {};
  feedback.apps = Object.fromEntries(Object.values(a.reportApps.resources.APP || {}).map(x => [x.key, x.name]));
}

async function fillReport(card, id, redraw) {
  card.hidden = false;
  card.replaceChildren();
  const report = feedback.rows.find(r => r.id === id);
  if (!report) {
    card.textContent = 'Could not open that report.';
    return;
  }
  const facts = el('dl');
  const rows = [
    ['Reporter', nameOf(report.added_by)],
    ['App', feedback.apps[report.app] || report.app],
    ['Page', report.page],
    ['Address', report.url],
    ['Browser', report.browser],
    ['Viewport', [report.viewport, report.screen].filter(Boolean).join(' of ')],
    ['Language', [report.language, report.time_zone].filter(Boolean).join(' · ')],
    ['Errors', report.errors],
  ];
  for (const [name, value] of rows) {
    if (value) {
      facts.append(el('dt', '', name), el('dd', '', value));
    }
  }
  card.append(el('h3', '', report.summary), el('div', 'said', report.details || 'No details given.'), facts);
  if (report.screenshot) {
    const link = el('a', 'shot');
    link.href = `/api/blob/${encodeURIComponent(report.id)}/screenshot`;
    link.target = '_blank';
    link.rel = 'noopener';
    const img = el('img');
    img.src = link.href;
    img.alt = 'The screenshot sent with this report';
    link.append(img);
    card.append(link);
  }
  if (report.status !== 'new') {
    card.append(el('div', 'hint', `${statusWords[report.status]} by ${nameOf(report.handled_by)}.`));
    if (report.issue) {
      const actions = el('div', 'card-actions');
      actions.append(issueLink(report, 'button', 'Open the issue'));
      card.append(actions);
    }
    return;
  }
  let draft;
  try {
    draft = await api('POST', '/api/do/report-draft', {report: report.id});
  } catch (err) {
    card.append(el('div', 'hint', `Could not draft the issue: ${err.message}`));
    return;
  }
  const hint = draft.filer
    ? `This is what ${draft.repo} will get, with the reporter's address already taken out. Edit it into an issue worth keeping, then file it.`
    : 'Filing on GitHub is not set up on this server, so this report can only be dismissed.';
  const title = field('input', 'Title', draft.title, {type: 'text', maxLength: 200});
  const body = field('textarea', 'Body', draft.body);
  const issueType = field('input', 'Type', draft.type, {type: 'text'});
  const labels = field('input', 'Labels', (draft.labels || []).join(', '), {type: 'text'});
  const status = el('span', 'save-status');
  const run = async (working, work) => {
    status.classList.remove('error');
    status.textContent = working;
    try {
      await work();
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
      return false;
    }
    await loadFeedback();
    await redraw();
    return true;
  };
  const fileButton = el('button', 'button', 'File on GitHub');
  fileButton.type = 'button';
  fileButton.disabled = !draft.filer;
  fileButton.addEventListener('click', async () => {
    fileButton.disabled = true;
    const done = await run('Filing…', () => api('POST', '/api/do/file-report', {
      report: report.id,
      title: title.input.value,
      body: body.input.value,
      type: issueType.input.value,
      labels: labels.input.value.split(',').map(l => l.trim()).filter(Boolean),
    }));
    fileButton.disabled = done;
  });
  const dismissButton = el('button', 'link-button danger', 'Dismiss');
  dismissButton.type = 'button';
  dismissButton.addEventListener('click', () => run('Dismissing…', () => {
    const now = new Date();
    const pad = n => String(n).padStart(2, '0');
    const handled = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
    return api('POST', '/api/q', {batch: [{set: report.id, cells: {status: 'dismissed', handled, handled_by: state.model.user.id}}]});
  }));
  const actions = el('div', 'report-actions');
  actions.append(fileButton, dismissButton, status);
  card.append(el('div', 'hint', hint), title.wrap, body.wrap, issueType.wrap, labels.wrap, actions);
}

function feedbackCard() {
  const wrap = el('div');
  const card = el('div', 'card');
  card.append(el('h2', '', 'Feedback'), el('div', 'hint', 'Everything reported through “Report a problem or idea”, newest first. Open one to read it, edit what it should say, and file it as an issue — the Heliosian bot opens it, so nobody’s name is on it. The reporter’s address and the page’s query string never reach GitHub.'));
  const filters = el('div', 'feedback-filters');
  const list = el('div');
  const detail = el('div', 'card report-detail');
  detail.hidden = true;
  const paint = () => {
    list.replaceChildren();
    const shown = feedback.rows.filter(r => !feedback.filter || r.status === feedback.filter);
    if (!shown.length) {
      list.append(el('div', 'hint', feedback.filter === 'new' ? 'Nothing waiting. Everything reported has been filed or dismissed.' : 'Nothing here.'));
    }
    for (const report of shown) {
      const row = el('div', 'report-row' + (feedback.open === report.id ? ' open' : ''));
      const body = el('div', 'body');
      body.append(el('div', 'summary', report.summary), el('div', 'meta', describe(report)));
      const stateCell = el('div', 'state');
      if (report.issue) {
        stateCell.append(issueLink(report, '', '#' + report.issue.split('/').pop()));
      }
      row.append(el('div', 'kind ' + report.kind, report.kind === 'bug' ? 'Problem' : 'Idea'), body, stateCell);
      row.addEventListener('click', e => {
        if (e.target.tagName !== 'A') {
          open(report.id);
        }
      });
      list.append(row);
    }
  };
  const open = id => {
    feedback.open = id;
    paint();
    fillReport(detail, id, render);
  };
  for (const [label, value] of [['New', 'new'], ['Filed', 'filed'], ['Dismissed', 'dismissed'], ['All', '']]) {
    const chip = el('button', 'filter-chip' + (feedback.filter === value ? ' active' : ''), label);
    chip.type = 'button';
    chip.addEventListener('click', () => {
      for (const other of filters.children) {
        other.classList.toggle('active', other === chip);
      }
      feedback.filter = value;
      paint();
    });
    filters.append(chip);
  }
  card.append(filters, list);
  paint();
  if (feedback.open) {
    fillReport(detail, feedback.open, render).then(() => detail.scrollIntoView({block: 'start'}));
  }
  wrap.append(card, detail);
  return wrap;
}

async function fillAdminPage(slot) {
  const control = [{key: 'admins', label: 'Admins', card: () => adminsCard({
    hint: 'Whoever is on this list can add, edit, and delete links and categories, and reach this page. Changes save immediately.',
    ...groupMembers(['in', {n: 'id'}, ['select', 'APP.admins', ['=', {n: 'key'}, 'home']]]),
  })}];
  if (state.model.superAdmin) {
    try {
      await loadFeedback();
    } catch (err) {
      slot.replaceWith(el('p', 'hint', `Could not load the reports: ${err.message}`));
      return;
    }
    control.push({key: 'feedback', label: 'Feedback', count: feedback.rows.filter(r => r.status === 'new').length, card: feedbackCard});
  }
  slot.replaceWith(buildAdminPage({appName: 'Heliosian', allowed: true, email: state.model.user.email, sections: [
    {title: 'Editing & Control', tabs: control},
  ]}));
}

export function adminPage() {
  if (!isAdmin()) {
    return buildAdminPage({appName: 'Heliosian', allowed: false, email: state.model.user.email, sections: []});
  }
  const slot = el('div');
  fillAdminPage(slot);
  return slot;
}
