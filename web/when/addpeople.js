import {el, button, toast, longToast} from '/elements.js';
import {popup} from '/modal.js';
import {api} from '/api.js';
import {query, act, create} from '/data.js';
import {personAdder, outsideAdder} from '/members.js';
import {ruleOptions, setRuleOptions, rules} from './inviteparts.js';
import {state} from './state.js';

export const outsideNote = 'Someone outside Helios - a coach, a grandparent, a friend - and their family. They get the email and the calendar invite with a page of their own to answer from, no sign-in needed, that shows the event and nothing of who else is coming.';

export function inviteWords(host, people) {
  const n = people.length;
  if (host) {
    return n === 1 ? `${people[0].name} added to the list.` : `${n} people added to the list.`;
  }
  return n === 1 ? `${people[0].name} invited - the invitation is on its way.` : `${n} people invited - the invitations are on their way.`;
}

export function guestAdders(e, view, done) {
  const onList = new Set([...(view.list || []), ...(view.coming || []), ...view.mine].filter(r => r.invited && r.email).map(r => r.email));
  const isOn = email => onList.has(email);
  const onAdd = close => async people => {
    await act('events', e.id, 'invite', {people});
    close();
    done(inviteWords(view.host, people));
  };
  return {
    person: close => personAdder({isOn, onAdd: onAdd(close), gradeColors: state.model.gradeColors}),
    outside: close => outsideAdder({isOn, onAdd: onAdd(close), note: outsideNote}),
  };
}

export async function openPicker(e, view, refresh, {tab = 'person'} = {}) {
  try {
    setRuleOptions(view.host ? await api('GET', '/api/when/invites/options') : ruleOptions);
  } catch (err) {
    toast(err.message);
    return;
  }
  const box = el('div', 'picker');
  const tabs = el('div', 'tabs');
  const panel = el('div', 'picker-panel');
  const kinds = view.host ? [['person', 'Add Person'], ['group', 'Add Group'], ['outside', 'Add Non-Helios']] : [['person', 'Add Person'], ['outside', 'Add Non-Helios']];
  let active = kinds.some(([key]) => key === tab) ? tab : 'person';
  for (const [key, label] of kinds) {
    const b = el('button', 'tab-button' + (key === active ? ' is-active' : ''), label);
    b.type = 'button';
    b.addEventListener('click', () => {
      active = key;
      for (const t of tabs.children) {
        t.classList.toggle('is-active', t === b);
      }
      paintPanel();
    });
    tabs.append(b);
  }
  let shut = null;
  const done = async words => {
    longToast(words);
    shut();
    refresh();
  };
  const adders = guestAdders(e, view, done);
  const paintPanel = () => {
    panel.replaceChildren();
    const made = active === 'group' ? groupPanel(e, done) : adders[active](() => {});
    panel.append(made);
    if (made.focus) {
      setTimeout(() => made.focus(), 0);
    }
  };
  box.append(tabs, panel);
  paintPanel();
  shut = popup('Add to the guest list', box, {wide: true}).shut;
}

function groupPanel(e, done) {
  const wrap = el('div', 'picker-group');
  const rule = rules.newRule('include');
  const holder = el('div', 'rule is-include picker-rule');
  const preview = el('div', 'audience-preview picker-preview');
  let timer = null;
  const askPreview = () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (!rules.ruleSaysSomething(rule)) {
        preview.textContent = '';
        return;
      }
      try {
        const answer = await api('POST', '/api/when/invites/preview', {id: e.id, rule});
        preview.textContent = answer.count ? `Picks out ${answer.count} ${answer.count === 1 ? 'person' : 'people'} now: ${answer.names.join(', ')}${answer.count > answer.names.length ? '…' : ''}` : 'Picks out nobody yet.';
      } catch (err) {
        preview.textContent = err.message;
      }
    }, 300);
  };
  holder.append(rules.ruleControls(rule, askPreview));
  wrap.append(holder, preview);
  const auto = el('label', 'picker-auto');
  const box = el('input');
  box.type = 'checkbox';
  box.checked = true;
  auto.append(box, el('span', '', 'Auto-invite new members'), el('small', '', 'Whoever comes to match this later - a family joining the classroom, a ticket sold - goes on the list either way. With this on they are sent their invitation too, once you have sent this group its invitations yourself; off, they wait in Pending for you.'));
  wrap.append(auto);
  const foot = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const add = button('Add group', 'plus', 'button', async () => {
    if (!rules.ruleSaysSomething(rule)) {
      status.textContent = 'Pick a role, some words, a classroom, a grade or a tag.';
      status.classList.add('error');
      return;
    }
    add.disabled = true;
    try {
      const made = await create('invite-groups', {id: e.id, rule, auto: box.checked});
      const read = await query('/api/invite-groups/' + encodeURIComponent(made.id));
      const added = read.get(read.result).count;
      done(`Group added, with ${added} ${added === 1 ? 'person' : 'people'} on the list now.`);
    } catch (err) {
      status.textContent = err.message;
      status.classList.add('error');
      add.disabled = false;
    }
  });
  foot.append(add, status);
  wrap.append(foot);
  return wrap;
}
