import {state, me, sourceWords, dayType, dayTypeName, eventDates, isParty, eventImage, parseDate, monthLabel, monthOf, answerOf, answer} from '../state.js';
import {dayTypeClass} from '/daytype.js';
import {render as markdown} from '/markdown.js';
import {el, svg, button, toast, longToast, copyText} from '/elements.js';
import {dateCard, googleCalendarLink} from '/datecard.js';
import {uploadImage, openImageSearch, imageSearchOn} from '../imagecontrol.js';
import {heroImageBar, detailHero} from '/heroimage.js';
import {openPersonCard} from '/personcard.js';
import {setTitle} from '/shell.js';
import {load} from '/router.js';
import {setPicture, setStatus} from '../writes.js';
import {openGuestForm} from '../guestpopups.js';
import {answerIcon} from '../inviteparts.js';
import {personTile} from '/people.js';
import {audienceChips, blocks} from '../events.js';
import {fetchInvites, familyBand, familyAnswered, comingCard, guestListSection, inviteHostCall, flyerCard, addFlyerLink, openEditor, hostsRow, rsvpRow, inviteCall, openGuestSettings} from '../invites.js';

function hero(e) {
  return detailHero({
    imageUrl: eventImage(e), title: e.title, path: e.path,
    stamp: dateCard({start: e.start, end: e.end, allDay: e.allDay, location: e.location, add: googleCalendarLink({title: e.title, start: e.start, end: e.end, allDay: e.allDay, location: e.location || '', details: [e.description, location.origin + e.path].filter(Boolean).join('\n\n')})}),
    extras: e.link ? [linkedBadge(e)] : [],
    edit: e.can.edit || e.can.image ? imageEdit(e) : null,
  });
}

function imported(e) {
  return e.source === 'google' || e.source === 'pdf';
}

function imageEdit(e) {
  const save = async image => {
    try {
      await setPicture(e, image);
    } catch (err) {
      toast(err.message);
      return;
    }
    toast(image ? 'Picture saved' : 'Picture removed');
    await load();
  };
  return {image: e.image, tools: {uploadImage, imageSearchOn, openImageSearch}, save};
}

function imageBar(e) {
  const edit = imageEdit(e);
  return heroImageBar({image: edit.image, imageUrl: eventImage(e), query: e.title, tools: edit.tools, save: edit.save});
}

let editorView = null;
let keepAsk = null;

export function eventPage(e) {
  editorView = null;
  setTitle(e.title);
  const page = el('div', 'event-page');
  const top = el('div', 'detail-top');
  const tools = el('div', 'detail-tools');
  const back = el('a', 'detail-back');
  back.href = '/day/' + eventDates(e)[0];
  back.setAttribute('data-link', '');
  back.append(svg('chevron-left'), el('span', '', monthLabel(monthOf(eventDates(e)[0]))));
  top.append(back);
  if (e.can.edit || e.can.correct) {
    const edit = button('Edit Event', 'edit', 'button button-small button-secondary', async () => {
      const {openEditor} = await import('../invites.js');
      openEditor(e, editorView, async () => {
        await load();
      });
    });
    edit.classList.add('detail-edit');
    tools.append(edit);
  }
  top.append(tools);
  page.append(top, hero(e));
  if (e.cancelled) {
    const band = el('div', 'pending-band is-declined');
    const words = el('div', 'pending-words');
    words.append(el('div', 'pending-title', 'Cancelled'), el('div', 'pending-lead', 'The hosts called this event off. It is no longer on anyone\u2019s calendar.'));
    band.append(svg('close'), words);
    page.append(band);
  } else if (e.pending || e.declined) {
    page.append(pendingBand(e));
  } else if (e.sharing !== 'Public') {
    page.append(inviteBand(e));
  }

  const cols = el('div', 'detail-cols');
  const main = el('div', 'detail-main');
  const marks = el('div', 'detail-marks');
  if (e.dayType) {
    const name = dayTypeName(e.dayType);
    marks.append(el('span', 'chip chip-day ' + dayTypeClass(name), name));
  }
  if (!((e.sharing !== 'Public' || e.cancelled) && !e.classrooms.length)) {
    marks.append(audienceChips(e));
  }
  if (e.source === 'sheet') {
    const status = e.cancelled ? ['Cancelled', 'chip-status-declined'] : e.declined ? ['Declined', 'chip-status-declined'] : e.pending ? ['Pending approval', 'chip-status-pending'] : e.sharing === 'Link' ? ['Anyone with the link', 'chip-status-link'] : e.sharing === 'Invite Only' ? ['Invite only', 'chip-status-link'] : ['Public', 'chip-status-public'];
    marks.append(el('span', 'chip chip-status ' + status[1], status[0]));
  }
  main.append(marks);
  main.append(el('h1', 'detail-title', e.title));
  if (e.description) {
    const text = el('div', 'markdown');
    text.append(markdown(e.description));
    main.append(text);
  }
  const type = e.dayType ? dayType(e.dayType) : null;
  if (type && e.allDay) {
    const card = el('div', 'plan-card ' + dayTypeClass(type.name));
    const head = el('div', 'plan-head');
    head.append(el('div', 'plan-type', type.name));
    card.append(head);
    if (type.blocks.length) {
      card.append(blocks(type));
    }
    main.append(el('h2', 'section-title', 'The day for ' + (e.classrooms.length ? e.classrooms.join(', ') : 'everyone')));
    main.append(card);
  }
  const ask = el('div', 'detail-ask');
  const mineCard = el('div', 'side-card rsvp-compact');
  mineCard.hidden = true;
  const guest = {};
  if (!e.cancelled) {
    ask.append(rsvpCard(e, mineCard, guest));
  }
  main.append(ask);
  cols.append(main);

  const side = el('div', 'detail-side');
  const when = el('div', 'side-card side-card-facts');
  side.append(when);
  side.append(mineCard);
  const answered = el('div', 'detail-answered');
  side.append(answered);
  if (e.source !== 'celebrate' && e.source !== 'team') {
    side.append(sourceCard(e));
  }
  cols.append(side);
  page.append(cols);
  fillInvites(e, ask, answered, {info: when, linkedLine: page.querySelector('.hero-linked-line span'), guest});
  return page;
}

async function fillInvites(e, ask, answered, {info, linkedLine, guest}) {
  if (e.cancelled || (e.source !== 'sheet' && !e.link && !imported(e))) {
    return;
  }
  const fetched = async () => {
    try {
      return await fetchInvites(e);
    } catch (err) {
      answered.replaceChildren(el('p', 'save-status error', 'Couldn’t load the guest list: ' + err.message));
      return null;
    }
  };
  let view = await fetched();
  if (!view || !ask.isConnected) {
    return;
  }
  const refresh = async () => {
    keepAsk = ask.querySelector('.rsvp-panel') ? e.id : null;
    await load();
  };
  const keep = keepAsk === e.id;
  keepAsk = null;
  if (view.mine.length) {
    ask.closest('.event-page')?.querySelector('.rsvp-compact')?.remove();
    const self = view.mine.find(r => r.key === me().id && !r.guestOf);
    const done = self ? Boolean(self.answer) : familyAnswered(view);
    ask.replaceChildren(done && !keep ? '' : familyBand(e, view, refresh));
  } else if ((view.guests || view.host) && (e.sharing === 'Public' || e.sharing === 'Link') && !isParty(e) && guest.repaint) {
    guest.add = () => openGuestForm(e, me().id, refresh);
    guest.repaint();
  }
  editorView = view;
  if (view.host) {
    const tools = ask.closest('.event-page')?.querySelector('.detail-tools');
    if (tools && !tools.querySelector('.detail-edit')) {
      const edit = button('Edit Event', 'edit', 'button button-small button-secondary', () => openEditor(e, view, refresh));
      edit.classList.add('detail-edit');
      tools.append(edit);
    }
    if (tools && !tools.querySelector('.detail-guest-settings')) {
      const settings = button('Guest List Settings', 'gear', 'button button-small button-secondary', () => openGuestSettings(e, editorView, refresh));
      settings.classList.add('detail-guest-settings');
      tools.prepend(settings);
    }
    const heroWrap = ask.closest('.event-page')?.querySelector('.detail-hero');
    if ((e.source === 'sheet' || imported(e)) && heroWrap && !heroWrap.querySelector('.hero-image-bar')) {
      heroWrap.append(imageBar(e));
    }
  }
  const listed = view.host && view.list.some(r => r.invited);
  const call = !view.host || listed ? inviteCall(e, view, refresh) : null;
  if (call) {
    ask.append(call);
  }
  const invited = (view.host ? view.list : view.coming || []).some(r => r.invited);
  if (view.coming && invited) {
    ask.append(comingCard(e, view, refresh));
  }
  if (view.host) {
    if (listed) {
      answered.replaceChildren(guestListSection(e, view, refresh));
    } else {
      ask.append(inviteHostCall(e, view, refresh));
      answered.replaceChildren();
    }
  }
  if (!invited) {
    const card = rsvpsCard(view.list.filter(r => r.answer), view.host);
    if (card) {
      answered.replaceChildren(card);
    }
  }
  if ((view.hosts.length || view.host) && (view.host || !view.hostsHidden)) {
    info.append(hostsRow(e, view, refresh));
  }
  if (view.mine.length) {
    info.append(rsvpRow(e, view, refresh));
  }
  const flyer = flyerCard(e, view, refresh);
  if (flyer) {
    info.after(flyer);
  }
  const addFlyer = addFlyerLink(e, view, refresh);
  if (addFlyer) {
    info.closest('.detail-side').append(addFlyer);
  }
  const hosts = view.hosts.map(h => h.name).filter(Boolean);
  if (linkedLine && hosts.length) {
    linkedLine.textContent = (isParty(e) ? 'Hosted by ' : 'Chaired by ') + (hosts.length > 1 ? hosts.slice(0, -1).join(', ') + ' and ' + hosts[hosts.length - 1] : hosts[0]);
  }
}

function stampWords(stamp) {
  const day = (stamp || '').slice(0, 10);
  return parseDate(day) ? parseDate(day).toLocaleDateString('en-US', {month: 'short', day: 'numeric', year: 'numeric'}) : stamp;
}

function outLink(href, words) {
  const a = el('a', 'link-button');
  a.href = href;
  a.target = '_blank';
  a.rel = 'noopener';
  a.append(svg('open'), el('span', '', words));
  return a;
}

function rsvpCard(e, slot, guest) {
  const band = el('div', 'rsvp-band');
  let changing = false;
  const guestLink = links => {
    if (!guest.add) {
      return;
    }
    const b = el('button', 'rsvp-clear', 'Add a guest');
    b.type = 'button';
    b.addEventListener('click', guest.add);
    links.append(b);
  };
  const paint = () => {
    band.replaceChildren();
    slot.replaceChildren();
    const word = answerOf(e);
    const settled = !isParty(e) && (word === 'yes' || word === 'no' || word === 'maybe');
    band.hidden = settled;
    slot.hidden = !settled;
    const say = async next => {
      try {
        await answer(e, next);
        changing = false;
        toast(next === 'yes' ? 'A calendar invite is on its way to your email' : next === 'no' ? 'Marked as not going' : next === 'maybe' ? 'Marked as maybe' : next === 'hidden' ? 'Hidden - it shows on the month in gray' : word === 'hidden' ? 'Shown again' : 'Answer cleared');
        paint();
      } catch (err) {
        toast(err.message);
      }
    };
    const links = el('div', 'rsvp-links');
    const small = (text, next) => {
      const b = el('button', 'rsvp-clear', text);
      b.type = 'button';
      b.addEventListener('click', () => say(next));
      links.append(b);
    };
    if (word === 'yes' || word === 'no' || word === 'maybe') {
      small('Clear my RSVP', '');
    }
    if (word === 'hidden') {
      small('Show event', '');
    } else {
      small('Hide event', 'hidden');
    }
    const answers = () => [
      button('Yes', answerIcon('yes'), 'button rsvp-yes' + (word === 'yes' ? ' is-on' : ''), () => say(word === 'yes' ? '' : 'yes')),
      button('No', answerIcon('no'), 'button rsvp-no' + (word === 'no' ? ' is-on' : ''), () => say(word === 'no' ? '' : 'no')),
    ];
    if (settled) {
      const row = el('div', 'side-row');
      const icon = el('div', 'side-icon');
      icon.append(svg('calcheck'));
      const body = el('div', 'side-row-body');
      body.append(el('div', 'side-title', 'My RSVP'));
      if (changing) {
        const buttons = el('div', 'rsvp-compact-buttons');
        buttons.append(...answers());
        const back = el('button', 'rsvp-clear', 'Cancel');
        back.type = 'button';
        back.addEventListener('click', () => {
          changing = false;
          paint();
        });
        links.replaceChildren(links.firstChild, back);
        body.append(buttons, links);
      } else {
        const said = el('div', 'invite-said rsvp-compact-said is-' + word);
        const mark = el('span', 'invite-said-mark');
        mark.append(svg(answerIcon(word)));
        said.append(mark, el('strong', '', word === 'yes' ? 'You\u2019re going' : word === 'maybe' ? 'Maybe' : 'Not going'));
        body.append(said);
        if (word === 'yes') {
          body.append(el('div', 'side-line rsvp-compact-note', 'The invite is in your email.'));
        }
        const change = el('button', 'rsvp-clear', 'Change response');
        change.type = 'button';
        change.addEventListener('click', () => {
          changing = true;
          paint();
        });
        links.replaceChildren(change, links.lastChild);
        guestLink(links);
        body.append(links);
      }
      row.append(icon, body);
      slot.append(row);
      return;
    }
    const art = el('div', 'rsvp-art');
    art.append(svg('calcheck'));
    const words = el('div', 'rsvp-words');
    const buttons = el('div', 'rsvp-buttons');
    let note = '';
    if (isParty(e)) {
      const held = (e.minePeople || []).some(p => p.note !== 'waitlisted');
      if (held) {
        words.append(el('div', 'rsvp-title', word === 'yes' ? 'On your calendar' : 'Your household has tickets'), el('div', 'rsvp-lead', word === 'yes' ? 'The invite is in your email.' : 'Want it on your own calendar?'));
        buttons.append(button(word === 'yes' ? 'Invite sent' : 'Add to my calendar', word === 'yes' ? 'check' : 'calendar', 'button rsvp-yes' + (word === 'yes' ? ' is-on' : ''), () => say('yes')));
        note = word === 'yes' ? 'Click again to send it once more.' : 'Add to my calendar emails you a calendar invite.';
      } else {
        words.append(el('div', 'rsvp-title', 'No tickets yet'), el('div', 'rsvp-lead', 'Tickets are on the party page.'));
        const add = el('a', 'button rsvp-yes' + (e.availability === 'available' ? ' is-on' : ' is-off'));
        add.href = e.link;
        add.append(svg('ticket'), el('span', '', e.availability === 'available' ? 'Add Ticket' : e.call || 'See the party'));
        buttons.append(add);
      }
    } else {
      words.append(el('div', 'rsvp-title', word === 'hidden' ? 'Hidden' : 'Are you going?'), el('div', 'rsvp-lead', word === 'hidden' ? 'On the month in gray.' : 'Yes sends you a calendar invite.'));
      buttons.append(...answers());
    }
    if (note) {
      words.append(el('div', 'rsvp-note', note));
    }
    if (!isParty(e)) {
      guestLink(links);
    }
    const side = el('div', 'rsvp-side');
    side.append(buttons, links);
    const row = el('div', 'rsvp-band-row');
    row.append(art, words, side);
    band.append(row);
  };
  guest.repaint = paint;
  paint();
  return band;
}

function inviteBand(e) {
  const band = el('div', 'pending-band is-invite');
  const words = el('div', 'pending-words');
  const mine = e.hosted;
  if (e.sharing === 'Invite Only') {
    words.append(el('div', 'pending-title', 'Invite only'), el('div', 'pending-lead', mine ? 'Only the people you invite can open this event, and their answers put it on their calendars.' : 'You were invited. Your answer below puts it on your calendar.'));
    band.append(words);
    return band;
  }
  words.append(el('div', 'pending-title', 'Anyone with the link'), el('div', 'pending-lead', mine ? 'On the calendar of the people you invite, and of anyone you send this link to who answers.' : 'You were invited, or sent this link. Your answer below puts it on your calendar.'));
  band.append(svg('link'), words);
  const url = location.origin + e.path;
  band.append(button('Copy link', 'copy', 'button button-small', () => copyText(url, 'Link copied')));
  return band;
}

function pendingBand(e) {
  const band = el('div', 'pending-band' + (e.declined ? ' is-declined' : ''));
  const words = el('div', 'pending-words');
  const who = state.model.names && state.model.names[e.addedBy] ? state.model.names[e.addedBy] : e.addedBy;
  const mine = e.addedBy === me().id;
  const decides = e.can.approve && !mine;
  if (e.declined) {
    words.append(el('div', 'pending-title', 'Declined'), el('div', 'pending-lead', mine ? 'An admin declined this event, so it is not on the calendar. You can still edit it; an admin can approve it later.' : decides ? `Shared by ${who} and declined. Approve it to put it on the calendar after all.` : `Shared by ${who}. An admin declined it, so it is not on the calendar.`));
  } else {
    words.append(el('div', 'pending-title', 'Waiting for approval'), el('div', 'pending-lead', mine ? 'You shared this event. An admin will approve it onto the calendar; until then it is shared by link, so anyone you send the link to can open it.' : decides ? `Shared by ${who}. Approve it onto the calendar, or decline it.` : `Shared by ${who}. It goes on the calendar once an admin approves it.`));
  }
  band.append(svg(e.declined ? 'close' : 'clock'), words);
  if (e.can.approve) {
    const actions = el('div', 'pending-actions');
    const decide = async (action, done) => {
      try {
        await setStatus(e, action === 'approve' ? 'open' : 'closed');
      } catch (err) {
        toast(err.message);
        return;
      }
      toast(done);
      await load();
    };
    actions.append(button('Approve', 'check', 'button button-small', () => decide('approve', 'On the calendar')));
    if (e.can.decline && !e.declined) {
      actions.append(button('Decline', 'close', 'button button-secondary button-small', () => {
        if (confirm(`Decline ${e.title}? It comes off the calendar; you can approve it later.`)) {
          decide('decline', 'Declined');
        }
      }));
    }
    band.append(actions);
  }
  return band;
}

function rsvpsCard(rows, host) {
  if (!rows.length && !host) {
    return null;
  }
  const card = el('div', 'side-card rsvps-card');
  card.append(el('div', 'side-title', 'RSVPs'));
  if (!rows.length) {
    card.append(el('div', 'side-line', 'Nobody has answered yet.'));
    return card;
  }
  const yes = rows.filter(r => r.answer === 'yes');
  const no = rows.filter(r => r.answer === 'no');
  for (const [label, people] of [['Yes', yes], ['No', no]]) {
    if (!people.length) {
      continue;
    }
    card.append(el('div', 'rsvps-head', `${label} · ${people.length}`));
    const list = el('div', 'person-tiles');
    for (const p of people) {
      list.append(personTile(p, {
        onClick: () => openPersonCard(p),
        title: [p.name, p.line].filter(Boolean).join(' · '),
        gradeColors: state.model.gradeColors,
      }));
    }
    card.append(list);
  }
  return card;
}

function sourceCard(e) {
  const card = el('div', 'side-card side-card-help');
  const row = el('div', 'side-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('info'));
  const body = el('div', 'side-row-body');
  body.append(el('div', 'side-title', sourceWords(e)));
  const lines = [];
  switch (e.source) {
    case 'pdf':
      lines.push(`Read from the school\u2019s ${e.year ? e.year.replace('-', '\u2013') + ' ' : ''}year calendar.`);
      if (e.sourceTitle && e.sourceTitle !== e.title) {
        lines.push(`Printed there as \u201c${e.sourceTitle}\u201d.`);
      }
      break;
    case 'google':
      if (e.updated) {
        lines.push(`Last changed by the school on ${stampWords(e.updated)}.`);
      }
      if (e.sourceTitle && e.sourceTitle !== e.title) {
        lines.push(`Listed there as \u201c${e.sourceTitle}\u201d.`);
      }
      break;
    case 'sheet': {
      const names = state.model.names || {};
      const who = names[e.addedBy] || e.addedBy;
      if (who) {
        lines.push(`Added by ${who}${e.added ? ' on ' + stampWords(e.added) : ''}.`);
      }
      if (e.sourceNote) {
        lines.push(`Source: ${e.sourceNote}`);
      }
      break;
    }
  }
  for (const words of lines.filter(Boolean)) {
    body.append(el('div', 'side-line', words));
  }
  if (e.source === 'pdf') {
    body.append(outLink(e.sourceUrl, 'Open the school\u2019s calendar page'));
  } else if (e.source === 'google' && e.sourceUrl) {
    body.append(outLink(e.sourceUrl, 'Open in Google Calendar'));
  } else if (e.source === 'sheet' && e.sourceUrl) {
    const host = new URL(e.sourceUrl).hostname.replace(/^www\./, '');
    body.append(outLink(e.sourceUrl, host.includes('veracross') ? 'Open the source on Veracross' : `Open the source (${host})`));
    if (host.includes('veracross')) {
      body.append(el('div', 'side-line', 'Veracross asks for your parent portal login.'));
    }
  }
  row.append(icon, body);
  card.append(row);
  return card;
}

const standing = {
  available: 'Tickets available', waitlist: 'Full, and taking a waitlist', 'sold-out': 'Sold out',
  closed: 'Tickets are not on sale', past: 'This party has happened',
  open: 'Volunteers wanted', full: 'Every spot is taken', done: 'This event is done',
};

const mineStanding = {
  going: {celebrate: 'Your household holds tickets', team: 'Someone in your household has signed up'},
  waitlisted: {celebrate: 'Your household is on the waitlist'},
};

const linkedTitles = {celebrate: 'Fun(d)raiser party', team: 'HCA Team event'};

const seeWords = {celebrate: 'See the party', team: 'See the event'};

function linkedBadge(e) {
  const kind = isParty(e) ? 'celebrate' : 'team';
  const card = el('a', 'hero-linked' + (e.mine ? ' is-mine' : ''));
  card.href = e.link;
  card.title = e.mine ? seeWords[kind] : e.call || seeWords[kind];
  const mark = el('img', 'hero-linked-mark');
  mark.src = `/brand/apps/${kind}.png`;
  mark.alt = '';
  card.append(mark);
  const body = el('div', 'hero-linked-body');
  body.append(el('div', 'hero-linked-title', linkedTitles[kind]));
  const line = el('div', 'hero-linked-line');
  line.append(svg(kind === 'celebrate' ? 'ticket' : 'people'), el('span', '', linkedLineWords(e, kind)));
  body.append(line);
  card.append(body);
  return card;
}

function linkedLineWords(e, kind) {
  if (e.hostNames && e.hostNames.length) {
    const names = e.hostNames;
    return (kind === 'celebrate' ? 'Hosted by ' : 'Chaired by ') + (names.length > 1 ? names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1] : names[0]);
  }
  if (e.minePeople && e.minePeople.length) {
    return [...new Set(e.minePeople.map(p => p.name))].join(', ');
  }
  return e.mine ? `${e.mineWords} · ${mineStanding[e.mine][kind]}` : standing[e.availability] || (kind === 'celebrate' ? 'On Helios Celebrate' : 'On HCA-Team');
}
