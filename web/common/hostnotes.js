import {el, button, toast} from '/elements.js';
import {queries, rowsOf, write} from '/rail.js';
import {host} from '/guesthost.js';

function stamp() {
  const d = new Date();
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function when(added) {
  const d = new Date((added || '').replace(' ', 'T'));
  return isNaN(d) ? '' : d.toLocaleDateString('en-US', {month: 'short', day: 'numeric'}) + ', ' + d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
}

async function loadNotes(eventId) {
  const id = JSON.stringify(eventId);
  const a = await queries({
    notes: `(from COMMENT (where (= group ${id})) (order added asc) (columns person reply_to text added))`,
    writers: `(from PERSON (where (in id (select COMMENT.person (= group ${id})))) (columns name_show))`,
  });
  const names = Object.fromEntries(rowsOf(a.writers, 'PERSON').map(p => [p.id, p.name_show]));
  const all = rowsOf(a.notes, 'COMMENT').map(n => ({...n, name: names[n.person] || 'Someone'}));
  return all.filter(n => !n.reply_to).map(n => ({...n, replies: all.filter(r => r.reply_to === n.id)}));
}

function send(row) {
  return write([{insert: 'COMMENT', row: {...row, person: host.me().id, added: stamp()}}]);
}

function noteLine(n, mine, onRemove) {
  const line = el('div', 'host-note' + (n.reply_to ? ' is-reply' : ''));
  const head = el('div', 'host-note-head');
  head.append(el('strong', '', n.person === host.me().id ? 'You' : n.name), el('span', '', when(n.added)));
  if (mine) {
    const x = el('button', 'host-note-remove', '×');
    x.type = 'button';
    x.title = 'Take this back';
    x.addEventListener('click', onRemove);
    head.append(x);
  }
  line.append(head, el('p', 'host-note-text', n.text));
  return line;
}

function writeBox(placeholder, label, onSend) {
  const box = el('div', 'host-note-write');
  const text = el('textarea');
  text.rows = 2;
  text.maxLength = 2000;
  text.placeholder = placeholder;
  const go = button(label, 'send', 'button button-small', async () => {
    if (!text.value.trim()) {
      return;
    }
    go.disabled = true;
    try {
      await onSend(text.value.trim());
    } catch (err) {
      toast(err.message);
      go.disabled = false;
    }
  });
  box.append(text, go);
  return box;
}

export function notesPanel(e, hosting) {
  const panel = el('section', 'invite-panel host-notes');
  const paint = async () => {
    const threads = await loadNotes(e.id);
    panel.replaceChildren(el('h2', 'invite-panel-title', hosting ? 'Notes from guests' : 'Write the hosts'));
    if (hosting && !threads.length) {
      panel.append(el('p', 'invite-panel-note', 'When a guest writes you a note, it shows here for you to reply to. Only you and your co-hosts see them.'));
    }
    if (!hosting) {
      panel.append(el('p', 'invite-panel-note', 'A question or a note just for the hosts. Other guests don’t see it.'));
    }
    const remove = id => async () => {
      try {
        await write([{delete: id}]);
        await paint();
      } catch (err) {
        toast(err.message);
      }
    };
    for (const t of threads) {
      const thread = el('div', 'host-note-thread');
      thread.append(noteLine(t, t.person === host.me().id || hosting, remove(t.id)));
      for (const r of t.replies) {
        thread.append(noteLine(r, r.person === host.me().id, remove(r.id)));
      }
      if (hosting) {
        thread.append(writeBox(`Reply to ${t.name.split(' ')[0]}…`, 'Reply', async words => {
          await send({group: e.id, reply_to: t.id, text: words});
          await paint();
        }));
      }
      panel.append(thread);
    }
    if (!hosting) {
      panel.append(writeBox('Write the hosts…', 'Send', async words => {
        await send({group: e.id, text: words});
        toast('Sent to the hosts');
        await paint();
      }));
    }
  };
  paint().catch(err => panel.replaceChildren(el('p', 'save-status error', err.message)));
  return panel;
}
