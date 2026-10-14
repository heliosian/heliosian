import {me, answer} from './state.js';
import {el, svg, toast} from '/elements.js';
import {listed} from '/directory.js';
import {answerFor as answerOnList} from './guestlist.js';

export const answerWords = {yes: 'Yes', no: 'No'};

export function firstName(p) {
  return (p.name || p.email || '').split(' ')[0];
}

export function answerIcon(word) {
  return word === 'yes' ? 'check' : word === 'no' ? 'close' : 'info';
}

export async function answerFor(e, row, next) {
  if (row.key === me().id) {
    await answer(e, next);
    return;
  }
  await answerOnList(row, next);
}

export function answerButtons(row, e, onChange, {small = true} = {}) {
  const wrap = el('div', 'rsvp-mini' + (small ? ' is-small' : ''));
  const paint = () => {
    wrap.replaceChildren();
    for (const [word, label] of Object.entries(answerWords)) {
      const b = el('button', 'rsvp-mini-choice rsvp-mini-' + word + (row.answer === word ? ' is-on' : ''));
      b.type = 'button';
      b.append(svg(answerIcon(word)), el('span', '', label));
      b.disabled = !row.mine;
      b.addEventListener('click', async () => {
        const next = row.answer === word ? '' : word;
        try {
          await answerFor(e, row, next);
          row.answer = next;
          paint();
          onChange(row, next);
        } catch (err) {
          toast(err.message);
        }
      });
      wrap.append(b);
    }
  };
  paint();
  return wrap;
}

export function ticketWords(ticket) {
  return ticket === 'ticket' ? 'Purchased ticket' : ticket === 'free' ? 'Free ticket' : 'No ticket';
}

export function ticketDetail(ticket) {
  return ticket === 'ticket' ? 'Their household bought a ticket on Helios Celebrate.'
    : ticket === 'free' ? 'A place the hosts gave at no charge.'
      : 'Invited, with no ticket to the party. Tickets are on the party page on Helios Celebrate.';
}

export function stamp(s) {
  const day = (s || '').slice(0, 10);
  const d = day ? new Date(day + 'T12:00:00') : null;
  return d && !isNaN(d) ? d.toLocaleDateString('en-US', {month: 'short', day: 'numeric'}) : '';
}

function moment(s) {
  const d = s && s.length >= 16 ? new Date(s.slice(0, 10) + 'T' + s.slice(11, 16) + ':00') : null;
  return d && !isNaN(d) ? d.toLocaleDateString('en-US', {month: 'short', day: 'numeric'}) + ' ' + d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'}) : stamp(s);
}

export function answeredWords(r) {
  if (!r.answer) {
    return '';
  }
  const bits = [];
  if (r.answeredBy) {
    bits.push(`by ${r.answeredBy}`);
  }
  if (r.answeredAt) {
    bits.push(moment(r.answeredAt));
  }
  return bits.join(' · ');
}

export function pickerPeople(keep) {
  return async () => (await listed()).filter(keep);
}
