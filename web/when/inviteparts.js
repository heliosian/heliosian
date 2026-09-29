import {state, me, answer} from './state.js';
import {el, svg, avatar, toast} from '/elements.js';
import {act} from '/data.js';
import {listed} from '/directory.js';
import {rulesEditor} from '/rules.js';
import {gradeBadge} from '/people.js';

export const answerWords = {yes: 'Yes', maybe: 'Maybe', no: 'No'};

export function firstName(p) {
  return (p.name || p.email || '').split(' ')[0];
}

export function answerIcon(word) {
  return word === 'yes' ? 'check' : word === 'maybe' ? 'clock' : word === 'no' ? 'close' : 'info';
}

export async function answerFor(e, row, next) {
  if (row.key === me().email) {
    await answer(e, next);
    return;
  }
  await act('events', e.id, 'answer-for', {email: row.key, answer: next});
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

export function face(p, className) {
  const node = avatar(p, className || 'invite-face');
  if (p.grade) {
    node.append(gradeBadge(p.grade, state.model.gradeColors));
  }
  return node;
}

export function ticketWords(ticket) {
  return ticket === 'ticket' ? 'Purchased ticket' : ticket === 'free' ? 'Free ticket' : ticket === 'waitlist' ? 'On the waitlist' : 'No ticket';
}

export function ticketDetail(ticket) {
  return ticket === 'ticket' ? 'Their household bought a ticket on Helios Celebrate.'
    : ticket === 'free' ? 'A ticket the hosts gave at no charge.'
      : ticket === 'waitlist' ? 'Their household asked for a ticket; the party was full. They hold no ticket yet.'
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
  bits.push(r.answeredVia === 'calendar' ? 'from their calendar app' : 'on the page');
  if (r.answeredAt) {
    bits.push(moment(r.answeredAt));
  }
  return bits.join(' · ');
}

export function pickerPeople(keep) {
  return async () => (await listed()).filter(keep);
}

export let ruleOptions = null;

export function setRuleOptions(options) {
  ruleOptions = options;
}

export const rules = rulesEditor({
  options: () => ruleOptions,
  personName: () => '',
});

export function groupWords(g) {
  const options = ruleOptions || {lists: [], tags: []};
  const labels = t => {
    const named = options.tags.find(x => x.key === t) || options.lists.find(l => l.key === t);
    return named ? named.name : t;
  };
  return rules.ruleWords({...g.rule, tagLabels: g.rule.tags.map(labels)});
}
