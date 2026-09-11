import {pendingItems, activityPath, rootOf, longDate} from '../state.js';
import {el, link, svg, thumb} from '../dom.js';
import {setTitle} from '../chrome.js';

// Activities and roles anyone proposed that an admin has not yet let through.
// Reached from the toolbar rather than the opportunities page, since it is a
// queue to work off rather than a year to browse.
export function approvalsPage() {
  setTitle('Approval Needed');
  const page = el('div', 'list-page');
  page.append(el('h1', 'page-title', 'Approval Needed'));
  const panel = el('div', 'panel');
  const items = pendingItems();
  if (!items.length) {
    panel.append(el('div', 'panel-empty', 'Nothing is waiting for approval.'));
  }
  for (const {act} of items) {
    const root = rootOf(act);
    const row = link(activityPath(act), 'row is-link');
    row.append(thumb(act.imageUrl || root.imageUrl, act.title));
    const body = el('div', 'row-body');
    body.append(el('div', 'label', `Proposed by ${act.addedBy || 'someone'} on ${longDate(act.added)}`));
    const title = el('div', 'row-title', root === act ? act.title : root.title);
    if (root !== act) {
      title.append(el('span', 'chain', '▶'), el('span', '', act.title));
    }
    body.append(title);
    if (act.description) {
      body.append(el('div', 'row-text clamp', act.description));
    }
    row.append(body);
    const chevron = svg('chevron');
    chevron.classList.add('chevron');
    row.append(chevron);
    panel.append(row);
  }
  page.append(panel);
  return page;
}
