import {el, button, longToast} from '/elements.js';
import {popup} from '/modal.js';
import {personAdder, outsideAdder} from '/members.js';
import {ruleEditor, newRule, ruleSaysSomething} from '/listrules.js';
import {addPeople, addRule, previewRules} from './guestlist.js';
import {state} from './state.js';

export const outsideNote = 'Someone outside Helios - a coach, a grandparent, a friend - and their family. They get the email and the calendar invite with a page of their own to answer from, no sign-in needed, that shows the event and nothing of who else is coming. Everyone in a family answers for the others.';

export function inviteWords(host, people) {
  const n = people.length;
  if (host) {
    return n === 1 ? `${people[0].name} added to the list.` : `${n} people added to the list.`;
  }
  return n === 1 ? `${people[0].name} invited - the invitation is on its way.` : `${n} people invited - the invitations are on their way.`;
}

export function guestAdders(e, view, done) {
  const onList = new Set([...(view.list || []), ...view.mine].filter(r => r.invited && r.email).map(r => r.email));
  const isOn = email => onList.has(email);
  const onAdd = close => async people => {
    await addPeople(e, view, people);
    close();
    done(inviteWords(view.host, people));
  };
  return {
    person: close => personAdder({isOn, onAdd: onAdd(close), gradeColors: state.model.gradeColors}),
    outside: close => outsideAdder({isOn, onAdd: onAdd(close), note: outsideNote}),
  };
}

export function openPicker(e, view, refresh, {tab = 'person'} = {}) {
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
    const made = active === 'group' ? groupPanel(e, view, done) : adders[active](() => {});
    panel.append(made);
    if (made.focus) {
      setTimeout(() => made.focus(), 0);
    }
  };
  box.append(tabs, panel);
  paintPanel();
  shut = popup('Add to the guest list', box, {wide: true}).shut;
}

function groupPanel(e, view, done) {
  const wrap = el('div', 'picker-group');
  const rule = newRule();
  const preview = el('div', 'audience-preview picker-preview');
  let timer = null;
  const askPreview = () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (!ruleSaysSomething(rule)) {
        preview.textContent = '';
        return;
      }
      try {
        const answer = await previewRules(e, view, [rule]);
        const n = answer.ruleCounts[0] || 0;
        const names = answer.members.filter(m => m.reasons.some(r => r.rule === 0)).map(m => m.name).filter(Boolean).slice(0, 12);
        preview.textContent = n ? `Picks out ${n} ${n === 1 ? 'person' : 'people'} now: ${names.join(', ')}${n > names.length ? '…' : ''}` : 'Picks out nobody yet.';
      } catch (err) {
        preview.textContent = err.message;
      }
    }, 300);
  };
  wrap.append(ruleEditor(rule, askPreview), preview);
  wrap.append(el('small', 'hint', view.autoInvite ? 'Whoever comes to match this later goes on the list and is sent the invitation as they come.' : 'Whoever comes to match this later goes on the list, and waits in Pending for you to send.'));
  const foot = el('div', 'modal-actions');
  const status = el('span', 'save-status');
  const add = button('Add group', 'plus', 'button', async () => {
    if (!ruleSaysSomething(rule)) {
      status.textContent = 'Pick a group, a class, a grade or an activity.';
      status.classList.add('error');
      return;
    }
    add.disabled = true;
    try {
      const answer = await previewRules(e, view, [rule]);
      await addRule(e, view, rule);
      const added = answer.ruleCounts[0] || 0;
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
