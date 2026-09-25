import {pendingItems, activityPath, rootOf, longDate} from '../state.js';
import {el, link, svg, thumb, button} from '../dom.js';
import {setTitle} from '../chrome.js';
import {saveActivityFields} from '../edit.js';

// approvalButtons are an admin's answer to a suggestion without the hat:
// Approve opens it, Hide parks it where only editors see it. Nothing else of
// the admin's comes with them.
export function approvalButtons(act) {
  const approve = button('Approve', 'check', 'button button-small', () => saveActivityFields(act, {status: 'Open'}));
  approve.title = `Approve ${act.title}`;
  const hide = button('Hide', 'close', 'button button-secondary button-small', () => saveActivityFields(act, {status: 'Hidden'}));
  hide.title = `Hide ${act.title}`;
  return [approve, hide];
}

// Activities and roles anyone proposed that an admin has not yet let through.
// Reached from the toolbar rather than the opportunities page, since it is a
// queue to work off rather than a year to browse. It is the admin list's,
// Super Admin Mode or not, and so are its buttons.
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
    // button() swallows the click so the row's link does not fire underneath.
    const actions = el('div', 'row-actions');
    actions.append(...approvalButtons(act));
    const chevron = svg('chevron');
    chevron.classList.add('chevron');
    actions.append(chevron);
    row.append(actions);
    panel.append(row);
  }
  page.append(panel);
  return page;
}
