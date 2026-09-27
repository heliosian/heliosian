import {state} from './state.js';
import {el, svg, iconOf} from './dom.js';
import {adminPage as buildAdminPage, adminsCard} from '/admin.js';
import {api} from '/api.js';
import {render} from '/router.js';

const feedback = {rows: [], filter: 'New', canFile: false, repo: '', open: new URLSearchParams(location.search).get('report') || ''};

function categoriesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Categories'), el('div', 'hint', 'The sections of the front page, in this order, with their emoji and what each holds. Rename, re-mark, reorder and add them from the front page in Super Admin Mode, through Edit Categories in the account menu.'));
  const model = state.model;
  for (const category of model.categories) {
    const row = el('div', 'admin-row');
    const emoji = el('div', 'category-row-image');
    emoji.append(svg(iconOf(category)));
    let meta = '';
    if (category.style === 'events') {
      meta = `Upcoming events from HCA-Team · ${(model.upcoming || []).length} ahead`;
    } else if (category.style === 'apps') {
      meta = `The community apps · ${(model.apps || []).length} you see`;
    } else {
      const n = category.links.length;
      meta = `${category.style === 'cards' ? 'Feature cards' : 'Compact tiles'} · ${n} link${n === 1 ? '' : 's'}`;
    }
    const body = el('div', 'grow');
    body.append(el('div', 'category-row-title', category.title), el('div', 'category-row-meta', meta));
    row.append(emoji, body);
    card.append(row);
  }
  const edit = el('a', 'button', 'Edit on the front page');
  edit.href = '/';
  const actions = el('div', 'card-actions');
  actions.append(edit);
  card.append(actions);
  return card;
}

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
  const data = await api('GET', '/api/admin/feedback');
  feedback.rows = data.reports;
  feedback.canFile = data.canFile;
  feedback.repo = data.repo;
}

async function fillReport(card, id, redraw) {
  card.hidden = false;
  card.replaceChildren();
  let report;
  try {
    report = await api('GET', '/api/admin/feedback/' + encodeURIComponent(id));
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
    link.href = `/api/admin/feedback/${encodeURIComponent(id)}/screenshot`;
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
  const hint = feedback.canFile
    ? `This is what ${feedback.repo} will get, with the reporter's address already taken out. Edit it into an issue worth keeping, then file it.`
    : 'Filing on GitHub is not set up on this server, so this report can only be dismissed.';
  const title = field('input', 'Title', report.draft.title, {type: 'text', maxLength: 200});
  const body = field('textarea', 'Body', report.draft.body);
  const issueType = field('input', 'Type', report.draft.type, {type: 'text'});
  const labels = field('input', 'Labels', (report.draft.labels || []).join(', '), {type: 'text'});
  const status = el('span', 'save-status');
  const act = async (url, working, body) => {
    status.classList.remove('error');
    status.textContent = working;
    try {
      await api('POST', url, body);
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
  fileButton.disabled = !feedback.canFile;
  fileButton.addEventListener('click', async () => {
    fileButton.disabled = true;
    const done = await act(`/api/admin/feedback/${encodeURIComponent(id)}/file`, 'Filing…', {
      title: title.input.value,
      body: body.input.value,
      type: issueType.input.value,
      labels: labels.input.value.split(',').map(l => l.trim()).filter(Boolean),
    });
    fileButton.disabled = done;
  });
  const dismissButton = el('button', 'link-button danger', 'Dismiss');
  dismissButton.type = 'button';
  dismissButton.addEventListener('click', () => act(`/api/admin/feedback/${encodeURIComponent(id)}/dismiss`, 'Dismissing…'));
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
  let admin;
  try {
    admin = await api('GET', '/api/admin/state');
  } catch (err) {
    slot.replaceWith(el('p', 'hint', `Failed to load admin state: ${err.message}`));
    return;
  }
  const control = [{key: 'admins', label: 'Admins', card: () => adminsCard({hint: 'Whoever is on this list can add, edit, and delete links and categories, and reach this page. Changes save immediately.', people: () => admin.people})}];
  if (admin.isSuperAdmin) {
    try {
      await loadFeedback();
    } catch (err) {
      slot.replaceWith(el('p', 'hint', `Could not load the reports: ${err.message}`));
      return;
    }
    control.push({key: 'feedback', label: 'Feedback', count: feedback.rows.filter(r => r.status === 'New').length, card: feedbackCard});
  }
  slot.replaceWith(buildAdminPage({appName: 'Heliosian', allowed: true, email: state.model.user.email, sections: [
    {title: 'Display', tabs: [{key: 'categories', label: 'Categories', card: categoriesCard}]},
    {title: 'Editing & Control', tabs: control},
  ]}));
}

export function adminPage() {
  const user = state.model.user;
  if (!user.isAdmin) {
    return buildAdminPage({appName: 'Heliosian', allowed: false, email: user.email, sections: []});
  }
  const slot = el('div');
  fillAdminPage(slot);
  return slot;
}
