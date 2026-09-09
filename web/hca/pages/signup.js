import {state, years, activitiesIn, sortByStart, isPrevious, isAdmin, pendingItems, matches, activityPath, rolePath, longDate} from '../state.js';
import {el, link, svg, button, searchBox, toggle, tabs, thumb} from '../dom.js';
import {setTitle} from '../chrome.js';
import {activityRow} from '../cards.js';
import {openActivity} from '../edit.js';

let tab = 'current';
let query = '';

function pendingList() {
  const panel = el('div', 'panel');
  const items = pendingItems();
  if (!items.length) {
    panel.append(el('div', 'panel-empty', 'Nothing is waiting for approval.'));
  }
  for (const {act, role} of items) {
    const node = role || act;
    const row = link(role ? rolePath(act, role) : activityPath(act), 'row is-link');
    row.append(thumb(node.imageUrl || act.imageUrl, node.title));
    const body = el('div', 'row-body');
    body.append(el('div', 'label', `Proposed by ${node.addedBy || 'someone'} on ${longDate(node.added)}`));
    const title = el('div', 'row-title', act.title);
    if (role) {
      title.append(el('span', 'chain', '▶'), el('span', '', role.title));
    }
    body.append(title);
    if (node.description) {
      body.append(el('div', 'row-text clamp', node.description));
    }
    row.append(body);
    const chevron = svg('chevron');
    chevron.classList.add('chevron');
    row.append(chevron);
    panel.append(row);
  }
  return panel;
}

function yearList(year) {
  const root = el('div');
  const shown = sortByStart(activitiesIn(year)).filter(a => (state.showPrevious || !isPrevious(a)) && matches(a, query));
  let any = false;
  for (const category of state.model.categories) {
    const items = shown.filter(a => a.category === category.title);
    if (!items.length) {
      continue;
    }
    any = true;
    root.append(el('div', 'section-title', category.title));
    if (category.description) {
      root.append(el('div', 'section-note', category.description));
    }
    const panel = el('div', 'panel');
    for (const act of items) {
      panel.append(activityRow(act));
    }
    root.append(panel);
  }
  if (!any) {
    const panel = el('div', 'panel');
    panel.append(el('div', 'panel-empty', query ? 'Nothing matches.' : 'Nothing to sign up for yet.'));
    root.append(panel);
  }
  return root;
}

function yearContent(year, thisYear) {
  const content = el('div');
  content.append(el('div', 'year-title', year));
  const sub = el('div', 'year-sub');
  sub.append(el('h2', '', thisYear ? 'Sign Up for This School Year' : `Sign Up for ${year}`));
  const list = el('div');
  sub.append(searchBox('Search', q => {
    query = q;
    list.replaceChildren(yearList(year));
  }));
  content.append(sub);
  list.append(yearList(year));
  content.append(list);
  return content;
}

export function signUpPage(yearParam) {
  if (yearParam) {
    tab = yearParam === years().last ? 'last' : 'current';
  }
  const page = el('div');
  const head = el('div', 'page-head');
  head.append(el('h1', '', 'HCA Volunteer Portal: Help Needed!'));
  if (state.model.settings.intro) {
    head.append(el('p', 'intro', state.model.settings.intro));
  }
  const buttons = el('div', 'button-row');
  const expense = el('a', 'button button-secondary');
  expense.href = state.model.settings.expenseFormUrl;
  expense.target = '_blank';
  expense.rel = 'noopener';
  expense.append(svg('receipt'), el('span', '', 'Expense Form'));
  buttons.append(expense, button('Suggest an Idea', 'idea', 'button', () => openActivity(null, {category: 'Just an Idea'})));
  head.append(buttons);
  page.append(head);

  const band = el('div', 'band');
  const inner = el('div', 'band-inner');
  band.append(inner);
  const items = [{key: 'current', label: 'Current School Year'}, {key: 'last', label: 'Last Year'}];
  if (isAdmin()) {
    items.push({key: 'approval', label: `Approval Needed (${pendingItems().length})`});
  }
  const render = () => {
    inner.replaceChildren(tabs(items, tab, key => {
      tab = key;
      query = '';
      history.replaceState(null, '', '/');
      render();
    }));
    if (tab === 'approval') {
      setTitle('Approval Needed');
      inner.append(el('div', 'year-title', 'Approval Needed'), pendingList());
      return;
    }
    const year = yearParam || (tab === 'last' ? years().last : years().current);
    setTitle(year);
    inner.append(yearContent(year, year === years().current));
  };
  page.append(toggle('Show Previous Events', state.showPrevious, on => {
    state.showPrevious = on;
    render();
  }), band);
  render();
  return page;
}
