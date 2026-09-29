import {el, svg, avatar, toast} from '/elements.js';
import {openModal, closeModal} from '/modal.js';
import {tabbedFields} from '/tabs.js';
import {directory} from '/directory.js';
import {whoLink} from '/appswitch.js';

export async function openPersonCard(person, tabs = [], options = {}) {
  const dir = await directory();
  const info = person.email ? dir.result.map(dir.get).find(p => p.email === person.email) || null : null;
  const head = cardHead(person, info);
  if (!tabs.length) {
    const done = el('button', 'button button-secondary', 'Done');
    done.type = 'button';
    done.addEventListener('click', closeModal);
    openModal('', [head, cardContact(dir, person, info), cardFoot(person, info, done)], {...options, actions: false, wide: 'person'});
    return;
  }
  const panels = tabbedFields([
    {label: 'Contact', icon: svg('people'), fields: [cardContact(dir, person, info), cardFoot(person, info, null)]},
    ...tabs,
  ]);
  openModal('', [head, panels], {...options, wide: 'person'});
}

function cardHead(person, info) {
  const head = el('div', 'who-head');
  const name = (info && info.fullName) || person.name || person.email;
  const face = avatar({name, email: person.email, photoUrl: (info && info.heroPhotoUrl) || person.photoUrl}, 'who-face');
  const names = el('div', 'who-names');
  names.append(el('div', 'who-name', name));
  if (info && info.pronouns) {
    names.append(el('div', 'who-sub', info.pronouns));
  }
  const place = info
    ? info.isStudent
      ? [info.grade, info.classroom].filter(Boolean).join(' · ')
      : [info.jobTitle, info.department].filter(Boolean).join(' · ') || info.words
    : person.line || (person.email ? '' : 'Guest');
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
  if (person.email) {
    action('mail', 'Email', `mailto:${person.email}`);
  }
  const phone = info && info.phone ? info.phone : '';
  const digits = phone.replace(/[^+\d]/g, '');
  if (digits) {
    action('chat', 'Message', `sms:${digits}`);
    action('phone', 'Call', `tel:${digits}`);
  }
  if (person.email) {
    action('copy', 'Copy info', null, () => {
      const lines = [name, person.email, phone].filter(Boolean);
      navigator.clipboard.writeText(lines.join('\n')).then(() => toast('Contact info copied'), () => toast('Could not copy'));
    });
  }
  head.append(face, names, actions);
  return head;
}

function cardContact(dir, person, info) {
  const card = el('div', 'who-rows');
  let group = null;
  const row = (icon, label, value) => {
    if (!group) {
      group = el('div', 'who-group');
      card.append(group);
    }
    const r = el('div', 'who-row');
    r.append(svg(icon), el('span', 'who-label', label), typeof value === 'string' ? el('span', 'who-value', value) : value);
    group.append(r);
  };
  if (person.email) {
    row('mail', 'Email', person.email);
  }
  if (info && info.phone) {
    row('phone', 'Phone', info.phone);
  }
  const chips = list => {
    const wrap = el('div', 'who-card-chips');
    for (const p of list) {
      const chip = el('button', 'who-card-chip', p.grade ? `${p.fullName} (${p.grade})` : p.fullName);
      chip.type = 'button';
      chip.addEventListener('click', () => openPersonCard({email: p.email, name: p.fullName, photoUrl: p.heroPhotoUrl}));
      wrap.append(chip);
    }
    return wrap;
  };
  const partners = info ? dir.follow(info, 'partners').filter(Boolean) : [];
  const children = info ? dir.follow(info, 'children').filter(Boolean) : [];
  const parents = info && info.isStudent ? info.parentContactEmails || [] : [];
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

function cardFoot(person, info, done) {
  const foot = el('div', 'who-foot');
  if (info) {
    const profile = el('a', 'button', '');
    profile.append(svg('open'), el('span', '', 'Open Helios Who? Profile'));
    profile.href = whoLink(person.email);
    profile.target = '_blank';
    profile.rel = 'noopener';
    foot.append(profile);
  }
  if (done) {
    foot.append(done);
  }
  return foot.children.length ? foot : el('div');
}
