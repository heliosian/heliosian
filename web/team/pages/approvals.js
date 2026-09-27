import {pendingItems, activityPath, rootOf, longDate} from '../state.js';
import {el, link, svg, imageThumb, button} from '/elements.js';
import {setTitle} from '/shell.js';
import {saveActivityFields} from '../edit.js';

export function approvalButtons(act) {
  const approve = button('Approve', 'check', 'button button-small', () => saveActivityFields(act, {status: 'Open'}));
  approve.title = `Approve ${act.title}`;
  const hide = button('Hide', 'close', 'button button-secondary button-small', () => saveActivityFields(act, {status: 'Hidden'}));
  hide.title = `Hide ${act.title}`;
  return [approve, hide];
}

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
    row.append(imageThumb(act.imageUrl || root.imageUrl, act.title));
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
    const actions = el('div', 'row-actions');
    actions.append(...approvalButtons(act));
    const chevron = svg('chevron-right');
    chevron.classList.add('chevron');
    actions.append(chevron);
    row.append(actions);
    panel.append(row);
  }
  page.append(panel);
  return page;
}
