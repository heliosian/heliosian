import {el, svg, avatar, toast} from '/elements.js';
import {openModal, closeModal} from '/modal.js';
import {tabbedFields} from '/tabs.js';
import {personByEmail, emailOf, photoOf, placeOf, wordsOf, gradeOf, isStudent, relativesOf, profileLink} from '/directory.js';

export async function openPersonCard(person, tabs = [], options = {}) {
  const info = person.email ? await personByEmail(person.email) : null;
  const head = cardHead(person, info);
  if (!tabs.length) {
    const done = el('button', 'button button-secondary', 'Done');
    done.type = 'button';
    done.addEventListener('click', closeModal);
    openModal('', [head, cardContact(person, info), cardFoot(info, done)], {...options, actions: false, wide: 'person'});
    return;
  }
  const panels = tabbedFields([
    {label: 'Contact', icon: svg('people'), fields: [cardContact(person, info), cardFoot(info, options.actions === false ? doneButton() : null)]},
    ...tabs,
  ]);
  openModal('', [head, panels], {...options, wide: 'person'});
}

function cardHead(person, info) {
  const head = el('div', 'who-head');
  const name = (info && info.name_show) || person.name || person.email;
  const face = avatar({name, email: person.email, photoUrl: (info && photoOf(info)) || person.photoUrl}, 'who-face');
  const names = el('div', 'who-names');
  names.append(el('div', 'who-name', name));
  if (info && info.pronouns) {
    names.append(el('div', 'who-sub', info.pronouns));
  }
  const place = info ? placeOf(info) || wordsOf(info) : person.line || (person.email ? '' : 'Guest');
  if (place) {
    names.append(el('div', 'who-sub', place));
  }
  const actions = el('div', 'who-actions');
  const action = (icon, label, href, onClick) => {
    const a = el(href ? 'a' : 'button', 'who-action');
    if (href) {
      a.href = href;
    } else {
      a.type = 'button';
      a.addEventListener('click', onClick);
    }
    a.title = label;
    const round = el('span', 'icon-round');
    round.append(svg(icon));
    a.append(round, el('span', 'who-action-label', label));
    actions.append(a);
  };
  const phone = info && info.phone ? info.phone : '';
  if (person.email) {
    action('copy', 'Copy info', null, () => {
      const lines = [name, person.email, phone].filter(Boolean);
      navigator.clipboard.writeText(lines.join('\n')).then(() => toast('Contact info copied'), () => toast('Could not copy'));
    });
  }
  head.append(face, names, actions);
  return head;
}

function cardContact(person, info) {
  const card = el('div', 'who-rows');
  let group = null;
  const row = (icon, label, value, links = []) => {
    if (!group) {
      group = el('div', 'who-group');
      card.append(group);
    }
    const r = el('div', 'who-row');
    r.append(svg(icon), el('span', 'who-label', label), typeof value === 'string' ? el('span', 'who-value', value) : value);
    if (links.length) {
      const wrap = el('span', 'who-row-actions');
      for (const [linkIcon, title, href] of links) {
        const a = el('a', 'who-row-action');
        a.href = href;
        a.title = title;
        a.setAttribute('aria-label', title);
        a.append(svg(linkIcon));
        wrap.append(a);
      }
      r.append(wrap);
    }
    group.append(r);
  };
  if (person.email) {
    row('mail', 'Email', person.email, [['mail', 'Email', `mailto:${person.email}`]]);
  }
  if (info && info.phone) {
    const digits = info.phone.replace(/[^+\d]/g, '');
    row('phone', 'Phone', info.phone, digits ? [['chat', 'Message', `sms:${digits}`], ['phone', 'Call', `tel:${digits}`]] : []);
  }
  const chips = list => {
    const wrap = el('div', 'who-card-chips');
    for (const p of list) {
      const chip = el('button', 'who-card-chip', gradeOf(p) ? `${p.name_show} (${gradeOf(p)})` : p.name_show);
      chip.type = 'button';
      chip.addEventListener('click', () => openPersonCard({email: emailOf(p), name: p.name_show, photoUrl: photoOf(p)}));
      wrap.append(chip);
    }
    return wrap;
  };
  const partners = info ? relativesOf(info, 'partners') : [];
  const children = info ? relativesOf(info, 'children') : [];
  const parents = info && isStudent(info) ? relativesOf(info, 'parents').map(emailOf).filter(Boolean) : [];
  if (partners.length || children.length || parents.length) {
    group = null;
  }
  if (partners.length) {
    row('people', partners.length === 1 ? 'Partner' : 'Partners', chips(partners));
  }
  if (children.length) {
    row('person', children.length === 1 ? 'Child' : 'Children', chips(children));
  }
  if (parents.length) {
    const links = el('div');
    for (const e of parents) {
      const a = el('a', 'who-card-link', e);
      a.href = `mailto:${e}`;
      links.append(a);
    }
    row('people', 'Parents', links);
  }
  if (!card.children.length) {
    row('person', 'About', 'A guest, not in the directory.');
  }
  return card;
}

export function doneButton() {
  const done = el('button', 'button who-done', 'Done');
  done.type = 'button';
  done.addEventListener('click', closeModal);
  return done;
}

function cardFoot(info, done) {
  const foot = el('div', 'who-foot');
  if (info) {
    const profile = el('a', 'button', '');
    profile.append(svg('open'), el('span', '', 'Open Helios Who? Profile'));
    profile.href = profileLink(info);
    profile.target = '_blank';
    profile.rel = 'noopener';
    foot.append(profile);
  }
  if (done) {
    foot.append(done);
  }
  return foot.children.length ? foot : el('div');
}
