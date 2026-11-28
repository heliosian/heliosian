import {state} from '../state.js';
import {el} from '/elements.js';
import {setTitle} from '/shell.js';
import {notFound} from '/router.js';
import {asEvent, guestListWords, guestListShown} from '/invitation.js';
import {fetchInvites} from '/guestlist.js';
import {comingCard} from '/invites.js';
import {themedPage} from '../chrome.js';

export function guestsPage() {
  const inv = state.model.invitation;
  if (!inv || !guestListShown(inv)) {
    return notFound('That page');
  }
  setTitle(`${guestListWords(inv)} · ${inv.name}`);
  const page = themedPage();
  const e = asEvent(inv);
  const show = () => fetchInvites(e).then(view => {
    page.replaceChildren(comingCard(e, view, show, `${inv.name} · ${guestListWords(inv)}`));
  }, err => {
    page.replaceChildren(el('p', 'save-status error', 'Couldn’t load the guest list: ' + err.message));
  });
  show();
  return page;
}
