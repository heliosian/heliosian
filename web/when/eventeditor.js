import {el, button} from '/elements.js';
import {popup} from '/modal.js';
import {offerUpdate} from '/invites.js';
import {deleteInvitation, openCancel} from '/guestpopups.js';

export async function openEditor(e, view, refresh) {
  if (!e.can.edit && !e.can.correct) {
    return;
  }
  const {eventForm} = await import('./eventform.js');
  let shut = null;
  const box = el('div', 'editor');
  box.append(eventForm({edit: e, override: e.can.correct && e.source !== 'sheet', more: [], onDone: async (ids, changed) => {
    shut();
    await refresh();
    if (changed && changed.length) {
      offerUpdate(e, changed);
    }
  }}));
  if (view && view.host && e.source === 'sheet' && !e.cancelled) {
    const foot = el('div', 'editor-danger');
    foot.append(view.sent
      ? button('Cancel event', 'close', 'link-button danger', () => {
        shut();
        openCancel(e, view, refresh);
      })
      : button('Delete event', 'trash', 'link-button danger', () => deleteInvitation(e)));
    box.append(foot);
  }
  shut = popup('Edit ' + e.title, box, {wide: true}).shut;
}
