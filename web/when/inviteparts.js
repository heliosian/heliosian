import {state, me, answer} from './state.js';
import {el, svg, avatar, toast} from '/elements.js';
import {api} from '/api.js';
import {rulesEditor} from '/rules.js';

export const answerWords = {yes: 'Yes', maybe: 'Maybe', no: 'No'};

export function firstName(p) {
  return (p.name || p.email || '').split(' ')[0];
}

export function answerButtons(row, e, onChange, {small = true} = {}) {
  const wrap = el('div', 'rsvp-mini' + (small ? ' is-small' : ''));
  const paint = () => {
    wrap.replaceChildren();
    for (const [word, label] of Object.entries(answerWords)) {
      const b = el('button', 'rsvp-mini-choice rsvp-mini-' + word + (row.answer === word ? ' is-on' : ''));
      b.type = 'button';
      b.append(svg(word === 'yes' ? 'check' : word === 'no' ? 'close' : 'clock'), el('span', '', label));
      b.disabled = !row.mine;
      b.addEventListener('click', async () => {
        const next = row.answer === word ? '' : word;
        try {
          if (row.key === me().email) {
            await answer(e, next);
          } else {
            await api('POST', '/api/when/invites/answer', {id: e.id, email: row.key, answer: next});
          }
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
    const badge = el('span', 'grade-badge', /^kindergarten$/i.test(p.grade) ? 'K' : p.grade.replace(/^grade\s*/i, ''));
    badge.title = p.grade;
    const color = (state.model.gradeColors || {})[p.grade];
    if (color) {
      badge.style.background = `color-mix(in srgb, ${color} 65%, black)`;
    }
    node.append(badge);
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

export function fetchPickerData(e) {
  return api('GET', '/api/when/invites/people?id=' + encodeURIComponent(e.id));
}

export function pickerPeople(e, keep) {
  let asked = null;
  return async () => {
    asked = asked || fetchPickerData(e).catch(err => {
      asked = null;
      throw err;
    });
    return (await asked).people.filter(keep).map(p => ({...p, name: p.name || p.email}));
  };
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
