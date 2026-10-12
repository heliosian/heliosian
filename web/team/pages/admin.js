import {state, allows, me, yearGroup, years} from '../state.js';
import {el, button} from '/elements.js';
import {categoryList, openSettings, openRedirect} from '../edit.js';
import {api} from '/api.js';
import {adminPage as buildAdminPage, adminsCard, groupMembers} from '/admin.js';

function categoriesCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Categories'));
  card.append(el('div', 'hint', 'The headings on the Opportunities page, in this order. Each event manages its own categories from its page.'));
  card.append(categoryList(null, null));
  return card;
}

function settingsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Settings'));
  card.append(el('div', 'hint', 'The expense form the Sign Up page links to, and the intro under its heading.'));
  const expense = el('div', 'admin-row');
  const expenseBody = el('div', 'grow');
  expenseBody.append(el('div', '', 'Expense form'), el('div', 'sub', state.model.settings.expenseFormUrl));
  expense.append(expenseBody);
  const intro = el('div', 'admin-row');
  const introBody = el('div', 'grow');
  introBody.append(el('div', '', 'Intro'), el('div', 'sub', state.model.settings.intro));
  intro.append(introBody);
  const add = el('div', 'add-row');
  add.append(button('Edit Settings', 'edit', 'button', openSettings));
  card.append(expense, intro, add);
  return card;
}

function notifyCard() {
  const year = yearGroup(years().current);
  if (!year || !year.notify) {
    const card = el('div', 'card');
    card.append(el('h2', '', 'Email notifications'), el('div', 'hint', `${years().current} names no group to hear of its sign-ups and additions.`));
    return card;
  }
  return adminsCard({
    title: 'Email notifications',
    hint: 'Everyone here gets an email for every sign-up and every addition this school year.',
    ...groupMembers([{'=': [{path: 'id'}, year.notify]}]),
  });
}

function adminsList() {
  return adminsCard({
    hint: 'Whoever is on this list can approve suggestions, edit any activity, and reach this page. Co-chairs edit their own activities without being here.',
    ...groupMembers([{in: [{path: 'id'}, {select: {from: 'APP', column: 'admins', where: [{'=': [{path: 'key'}, 'team']}]}}]}]),
  });
}

function redirectsCard() {
  const card = el('div', 'card');
  card.append(el('h2', '', 'Redirects'));
  card.append(el('div', 'hint', 'Old addresses on this site and where they now lead, followed before sign-in. Add one for a link from the old volunteer site, or any address that should land somewhere else.'));
  const rows = el('div');
  const status = el('span', 'save-status');
  api('QUERY', '/api/q', {from: 'REDIRECT', where: [{'=': [{path: 'app'}, 'team']}]})
    .then(answer => fillRedirects(rows, answer.result.map(id => answer.resources.REDIRECT[id])))
    .catch(err => {
      status.classList.add('error');
      status.textContent = err.message;
    });
  const add = el('div', 'add-row');
  add.append(button('Add Redirect', 'plus', 'button', () => openRedirect(null)));
  card.append(rows, add, status);
  return card;
}

function fillRedirects(rows, list) {
  const redirects = list.sort((a, b) => (b.added || '').localeCompare(a.added || '') || a.old.localeCompare(b.old));
  if (!redirects.length) {
    rows.append(el('div', 'hint', 'None yet.'));
  }
  for (const item of redirects) {
    const row = el('div', 'admin-row redirect-row');
    const body = el('div', 'grow');
    const line = el('div', 'redirect-line');
    line.append(el('code', 'redirect-old', item.old), el('span', 'redirect-arrow', '→'), el('code', 'redirect-new', item.new));
    body.append(line);
    if (item.added) {
      body.append(el('div', 'sub', item.added));
    }
    row.append(body, button('Edit', 'edit', 'button button-secondary button-small', () => openRedirect(item)));
    rows.append(row);
  }
}

const sections = [
  {title: 'Display', tabs: [
    {key: 'categories', label: 'Categories', card: categoriesCard},
    {key: 'settings', label: 'Settings', card: settingsCard},
  ]},
  {title: 'Editing & Control', tabs: [
    {key: 'notify', label: 'Email Notifications', card: notifyCard},
    {key: 'admins', label: 'Admins', card: adminsList},
  ]},
  {title: 'Addresses', tabs: [
    {key: 'redirects', label: 'Redirects', card: redirectsCard},
  ]},
];

export function adminPage() {
  return buildAdminPage({appName: 'HCA-Team', allowed: allows('team.configure'), email: me().email, sections});
}
