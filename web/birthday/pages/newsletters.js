import {state, isAdmin, year, staffFor, newsletterText, longDate} from '../state.js';
import {el, button, searchBox, menu, copyText} from '../dom.js';
import {setTitle} from '../chrome.js';
import {newsletterRow, emptyPanel} from '../cards.js';
import {openNewsletterDate, addNextWeek, removeNewsletterDate} from '../edit.js';

let query = '';

function copyIssue(date) {
  const lines = staffFor(date).filter(sv => sv.donation).map(sv => newsletterText(sv, sv.donation));
  if (!lines.length) {
    return copyText('', 'No donations recorded for this issue yet');
  }
  return copyText(lines.join('\n\n'), 'Copied the issue');
}

function list() {
  const dates = state.model.newsletterDates.filter(d => d >= year().start && d <= year().end).filter(d => !query || longDate(d).toLowerCase().includes(query));
  if (!dates.length) {
    return emptyPanel(query ? 'Nothing matches.' : 'No newsletter dates this year yet.');
  }
  const panel = el('div', 'panel');
  dates.forEach((date, i) => {
    const actions = [];
    if (isAdmin() && i === dates.length - 1) {
      actions.push(button('Add Next Week', 'bolt', 'button button-small', () => addNextWeek(date)));
    }
    const items = [{icon: 'copy', label: 'Copy this issue', onClick: () => copyIssue(date)}];
    if (isAdmin()) {
      items.push({icon: 'trash', label: 'Remove', danger: true, onClick: () => removeNewsletterDate(date)});
    }
    actions.push(menu(items));
    panel.append(newsletterRow(date, actions));
  });
  return panel;
}

export function newslettersPage() {
  setTitle('Newsletters');
  const page = el('div', 'list-page');
  const head = el('div', 'list-head');
  head.append(el('h1', '', 'Newsletter Dates'));
  const actions = el('div', 'row-actions');
  actions.append(searchBox('Search', q => {
    query = q;
    page.querySelector('.list').replaceChildren(list());
  }));
  if (isAdmin()) {
    actions.append(button('Add', 'plus', 'button', openNewsletterDate));
  }
  head.append(actions);
  page.append(head, el('div', 'section-note', `The ${year().current} birthday year runs ${longDate(year().start)} to ${longDate(year().end)}. A birthday lands in the first newsletter on or after it, or the last one of the year for a summer birthday.`));
  const wrap = el('div', 'list');
  wrap.append(list());
  page.append(wrap);
  return page;
}
