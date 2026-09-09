import {state, isAdmin, years, allYears, activitiesIn, sortByStart, matches} from '../state.js';
import {el, button, searchBox} from '../dom.js';
import {setTitle} from '../chrome.js';
import {activityRow} from '../cards.js';
import {openActivity, copyToNextYear} from '../edit.js';

let query = '';

function denied() {
  const page = el('div', 'list-page');
  page.append(el('h1', '', 'Admin access required'));
  return page;
}

function yearBlock(year) {
  const block = el('div', 'year-block');
  block.append(el('h2', '', year));
  const shown = sortByStart(activitiesIn(year)).filter(a => matches(a, query));
  let any = false;
  for (const category of state.model.categories) {
    const items = shown.filter(a => a.category === category.title);
    if (!items.length) {
      continue;
    }
    any = true;
    block.append(el('div', 'section-title', category.title));
    const panel = el('div', 'panel');
    for (const act of items) {
      const actions = [button('Edit', 'edit', 'button button-secondary button-small', () => openActivity(act))];
      if (year < years().next) {
        actions.unshift(button('Copy to Next Year', 'copy', 'button button-small', () => copyToNextYear(act)));
      }
      panel.append(activityRow(act, {noJoin: true, actions}));
    }
    block.append(panel);
  }
  if (!any) {
    const panel = el('div', 'panel');
    panel.append(el('div', 'panel-empty', 'Nothing here.'));
    block.append(panel);
  }
  return block;
}

export function allPage() {
  setTitle('All Activities');
  if (!isAdmin()) {
    return denied();
  }
  const page = el('div', 'list-page');
  const head = el('div', 'list-head');
  head.append(el('h1', '', 'All Activities'));
  const actions = el('div', 'row-actions');
  const list = el('div');
  const render = () => {
    list.replaceChildren();
    for (const year of allYears()) {
      list.append(yearBlock(year));
    }
  };
  actions.append(searchBox('Search', q => {
    query = q;
    render();
  }, true));
  actions.append(button('Add Activity', 'plus', 'button', () => openActivity(null, {})));
  head.append(actions);
  page.append(head, list);
  render();
  return page;
}
