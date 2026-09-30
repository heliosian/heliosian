import {state, isAdmin} from './state.js';
import {el} from '/elements.js';
import {adminPage as buildAdminPage, adminsCard, appAdmins} from '/admin.js';
import {query, act} from '/data.js';
import {render} from '/router.js';

const feedback = {rows: [], filter: 'New', open: new URLSearchParams(location.search).get('report') || ''};

function describe(report) {
  const parts = [report.appName || report.app, report.received].filter(Boolean);
  if (report.status === 'Filed') {
    parts.push(`filed by ${report.handledBy}`);
  } else if (report.status === 'Dismissed') {
    parts.push(`dismissed by ${report.handledBy}`);
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
  const read = await query('/api/feedback-reports');
  feedback.rows = read.result.map(read.get);
}

async function fillReport(card, id, redraw) {
  card.hidden = false;
  card.replaceChildren();
  let report;
  try {
    const read = await query('/api/feedback-reports/' + encodeURIComponent(id));
    report = read.get(read.result);
  } catch (err) {
    card.textContent = `Could not open that report: ${err.message}`;
    return;
  }
  const facts = el('dl');
  const rows = [
    ['Reporter', `${report.email} (${report.role})`],
    ['App', report.appName],
    ['Page', report.page],
    ['Address', report.url],
    ['Browser', report.browser],
    ['Viewport', [report.viewport, report.screen].filter(Boolean).join(' of ')],
    ['Language', [report.language, report.timezone].filter(Boolean).join(' · ')],
    ['Errors', (report.errors || []).join('\n')],
  ];
  for (const [name, value] of rows) {
    if (value) {
      facts.append(el('dt', '', name), el('dd', '', value));
    }
  }
  card.append(el('h3', '', report.summary), el('div', 'said', report.details || 'No details given.'), facts);
  if (report.screenshot) {
    const link = el('a', 'shot');
    link.href = `/api/admin/feedback/${encodeURIComponent(report.id)}/screenshot`;
    link.target = '_blank';
    link.rel = 'noopener';
    const img = el('img');
    img.src = link.href;
    img.alt = 'The screenshot sent with this report';
    link.append(img);
    card.append(link);
  }
  if (report.status !== 'New') {
    card.append(el('div', 'hint', `${report.status === 'Filed' ? 'Filed' : 'Dismissed'} by ${report.handledBy}.`));
    if (report.issue) {
      const actions = el('div', 'card-actions');
      actions.append(issueLink(report, 'button', 'Open the issue'));
      card.append(actions);
    }
    return;
  }
  const hint = report.can.file
    ? `This is what ${report.draft.repo} will get, with the reporter's address already taken out. Edit it into an issue worth keeping, then file it.`
    : 'Filing on GitHub is not set up on this server, so this report can only be dismissed.';
  const title = field('input', 'Title', report.draft.title, {type: 'text', maxLength: 200});
  const body = field('textarea', 'Body', report.draft.body);
  const issueType = field('input', 'Type', report.draft.type, {type: 'text'});
  const labels = field('input', 'Labels', (report.draft.labels || []).join(', '), {type: 'text'});
  const status = el('span', 'save-status');
  const run = async (action, working, body) => {
    status.classList.remove('error');
    status.textContent = working;
    try {
      await act('feedback-reports', report.id, action, body);
    } catch (err) {
      status.classList.add('error');
      status.textContent = err.message;
      return false;
    }
    await redraw();
    return true;
  };
  const fileButton = el('button', 'button', 'File on GitHub');
  fileButton.type = 'button';
  fileButton.disabled = !report.can.file;
  fileButton.addEventListener('click', async () => {
    fileButton.disabled = true;
    const done = await run('file', 'Filing…', {
      title: title.input.value,
      body: body.input.value,
      type: issueType.input.value,
      labels: labels.input.value.split(',').map(l => l.trim()).filter(Boolean),
    });
    fileButton.disabled = done;
  });
  const dismissButton = el('button', 'link-button danger', 'Dismiss');
  dismissButton.type = 'button';
  dismissButton.addEventListener('click', () => run('dismiss', 'Dismissing…'));
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
      list.append(el('div', 'hint', feedback.filter === 'New' ? 'Nothing waiting. Everything reported has been filed or dismissed.' : 'Nothing here.'));
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
  for (const [label, value] of [['New', 'New'], ['Filed', 'Filed'], ['Dismissed', 'Dismissed'], ['All', '']]) {
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
  const control = [{key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list can add, edit, and delete links and categories, and reach this page. Changes save immediately.', ...appAdmins('home')})}];
  if (state.model.allowances.includes('feedback')) {
    try {
      await loadFeedback();
    } catch (err) {
      slot.replaceWith(el('p', 'hint', `Could not load the reports: ${err.message}`));
      return;
    }
    control.push({key: 'feedback', label: 'Feedback', count: feedback.rows.filter(r => r.status === 'New').length, card: feedbackCard});
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
