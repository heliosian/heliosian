import {state, isAdmin, isSystemAdmin, postedAndHosting, calendarLink, sourceWords, dayType, eventDates, linkURL, isParty, eventImage, parseDate, monthLabel, monthOf, answerOf, answer, eventPath} from '../state.js';
import {dayTypeClass} from '/daytype.js';
import {el, link, svg, paragraphs, button, toast, avatar, popup, copyText} from '../dom.js';
import {dateCard} from '/datecard.js';
import {uploadImage, openImageSearch, imageSearchOn} from '../imagecontrol.js';
import {appOrigin} from '/toolbar.js';
import {setTitle} from '../chrome.js';
import {audienceChips, blocks} from '../events.js';
import {fetchInvites, familyBand, familyAnswered, comingCard, guestListSection, inviteHostCall, startParty, flyerCard, addFlyerLink, openEditor, hostsRow, rsvpRow} from '../invites.js';

// hero is the picture across the top of the page - the event's own, its
// first tag's, or the calendar's - with the date on a card at its corner:
// the weekday, the day, and the hours or the days it runs across.
function hero(e) {
  const wrap = el('div', 'detail-hero');
  const img = el('img');
  img.src = eventImage(e);
  img.alt = '';
  // A picture another app serves may be gone; the calendar's own stands in.
  img.addEventListener('error', () => {
    if (!img.src.endsWith('/brand/default-header.jpg')) {
      img.src = '/brand/default-header.jpg';
    }
  }, {once: true});
  wrap.append(img);
  wrap.append(dateCard(el, {start: e.start, end: e.end, allDay: e.allDay, location: e.location, add: calendarLink(e)}));
  // A linked event says which app runs it on a card at the banner's foot,
  // the way to its page there.
  if (e.link) {
    wrap.append(linkedBadge(e));
  }
  // The poster's or an admin's hand-added event takes a picture from the
  // strip across the banner's foot, the way the other apps' pages do.
  if ((e.source === 'sheet' && (isAdmin() || postedAndHosting(e))) || (imported(e) && isAdmin())) {
    wrap.append(heroImageBar(e));
  }
  return wrap;
}

// imported says an event comes from the school's calendars, which its
// admins host and correct.
function imported(e) {
  return e.source === 'google' || e.source === 'pdf';
}

// sheetTags are an event's tags as its sheet row holds them: without the
// built-in ones the model adds (Misc, Going and the apps').
function sheetTags(e) {
  const builtIn = new Set(state.model.tags.filter(t => t.builtIn).map(t => t.name));
  return e.tags.filter(t => !builtIn.has(t));
}

// heroImageBar is the strip across the foot of the banner: upload a
// picture, find one in the image libraries, or take the event's own off -
// each saved onto the event at once.
function heroImageBar(e) {
  const bar = el('div', 'hero-image-bar');
  const save = async image => {
    // An event the school's calendars bring keeps its picture in the
    // Overrides tab, over the school's version.
    const override = imported(e);
    const body = override ? {id: e.id, image} : {
      id: e.id, title: e.title, start: e.start, end: e.end, location: e.location || '', description: e.description || '',
      tags: sheetTags(e), keywords: e.keywords || [], source: e.sourceUrl || e.sourceNote || '', image, sharing: e.sharing,
    };
    const res = await fetch(override ? '/api/calendar/overrides/image' : '/api/calendar/events', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
    if (!res.ok) {
      toast(await res.text());
      return;
    }
    toast(image ? 'Picture saved' : 'Picture removed');
    const {load} = await import('../app.js');
    await load();
  };
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  file.addEventListener('change', async () => {
    if (!file.files.length) {
      return;
    }
    bar.replaceChildren(el('span', 'hero-image-status', 'Uploading\u2026'));
    try {
      const made = await uploadImage(file.files[0]);
      await save(made.name);
    } catch (err) {
      toast(err.message);
    }
  });
  const holder = el('div', 'hero-image-menu-holder');
  const toggle = el('button', 'hero-image-action');
  toggle.type = 'button';
  toggle.setAttribute('aria-haspopup', 'menu');
  toggle.append(svg('image'), el('span', '', e.image ? 'Replace image' : 'Add an image'), svg('down'));
  const menu = el('div', 'hero-image-menu');
  menu.hidden = true;
  const item = (icon, words, onClick) => {
    const b = el('button', 'hero-image-menu-item');
    b.type = 'button';
    b.append(svg(icon), el('span', '', words));
    b.addEventListener('click', () => {
      menu.hidden = true;
      onClick();
    });
    menu.append(b);
  };
  item('image', 'Upload image', () => file.click());
  if (imageSearchOn()) {
    item('search', 'Find an image', () => openImageSearch(e.title, picked => save(picked.name)));
  }
  toggle.addEventListener('click', ev => {
    ev.stopPropagation();
    menu.hidden = !menu.hidden;
    if (!menu.hidden) {
      document.addEventListener('click', () => {
        menu.hidden = true;
      }, {once: true});
    }
  });
  menu.addEventListener('click', ev => ev.stopPropagation());
  holder.append(toggle, menu, file);
  bar.append(holder);
  if (e.image) {
    const remove = el('button', 'hero-image-action');
    remove.type = 'button';
    remove.append(svg('trash'), el('span', '', 'Remove'));
    remove.addEventListener('click', () => save(''));
    bar.append(remove);
  }
  return bar;
}

// editorView is the guest list's view, once fetched, for Edit's
// Invitation tab.
let editorView = null;

export function eventPage(e) {
  editorView = null;
  setTitle(e.title);
  const page = el('div', 'event-page');
  const top = el('div', 'detail-top');
  const tools = el('div', 'detail-tools');
  const back = el('a', 'detail-back');
  back.href = '/day/' + eventDates(e)[0];
  back.setAttribute('data-link', '');
  // The way back is to the calendar open to the event's month, so it says so.
  back.append(svg('back'), el('span', '', monthLabel(monthOf(eventDates(e)[0]))));
  top.append(back);
  // A hand-added event is the poster's, or an admin's, to correct.
  // One Edit for the event and its invitation, in tabs; the invitation's
  // tab joins once the guest list is fetched (fillInvites), which also
  // gives a linked event's host their Edit.
  // An event the school's calendars bring is an admin's to correct, over
  // the school's version, in the Overrides tab.
  if ((imported(e) && isAdmin()) || (e.source === 'sheet' && (isAdmin() || postedAndHosting(e)))) {
    tools.append(button('Edit', 'pencil', 'button button-secondary button-small detail-edit', async () => {
      const {openEditor} = await import('../invites.js');
      openEditor(e, editorView, async () => {
        const {load} = await import('../app.js');
        await load();
      });
    }));
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
    marks.append(el('span', 'chip chip-day ' + dayTypeClass(e.dayType), e.dayType));
  }
  // An event by invitation or link with no classrooms is for whoever is
  // sent it, not Everyone.
  if (!((e.sharing !== 'Public' || e.cancelled) && !e.classrooms.length)) {
    marks.append(audienceChips(e));
  }
  // A hand-added event says how it is shared: public, by link, invite
  // only, waiting for approval, or declined.
  if (e.source === 'sheet') {
    const status = e.cancelled ? ['Cancelled', 'chip-status-declined'] : e.declined ? ['Declined', 'chip-status-declined'] : e.pending ? ['Pending approval', 'chip-status-pending'] : e.sharing === 'Link' ? ['Anyone with the link', 'chip-status-link'] : e.sharing === 'Invite Only' ? ['Invite only', 'chip-status-link'] : ['Public', 'chip-status-public'];
    marks.append(el('span', 'chip chip-status ' + status[1], status[0]));
  }
  main.append(marks);
  main.append(el('h1', 'detail-title', e.title));
  if (e.description) {
    main.append(paragraphs(e.description, 'prose detail-text', {lines: true}));
  }
  const type = e.dayType ? dayType(e.dayType) : null;
  if (type && e.allDay) {
    const card = el('div', 'plan-card ' + dayTypeClass(type.name));
    const head = el('div', 'plan-head');
    head.append(el('div', 'plan-type', type.name));
    card.append(head);
    if (type.blocks.length) {
      card.append(blocks(type));
    } else {
      card.append(el('div', 'plan-note', 'No dropoff, school, pickup, or aftercare.'));
    }
    main.append(el('h2', 'section-title', 'The day for ' + (e.classrooms.length ? e.classrooms.join(', ') : 'everyone')));
    main.append(card);
  }
  // The ask, under the event itself: are you going? - or, for someone
  // invited, who in the household is - and, for a host, the guest list.
  const ask = el('div', 'detail-ask');
  // The rail's quieter card for an answer already given (rsvpCard).
  const mineCard = el('div', 'side-card rsvp-compact');
  mineCard.hidden = true;
  if (!e.cancelled) {
    ask.append(rsvpCard(e, mineCard));
  }
  main.append(ask);
  cols.append(main);

  const side = el('div', 'detail-side');
  // The date, the hours and the place are on the banner's card; the
  // rail's card holds the hosts and My RSVP, once a guest list gives it
  // them (fillInvites), and hides while empty.
  const when = el('div', 'side-card side-card-facts');
  side.append(when);
  // The viewer's own answer, under the hosts.
  side.append(mineCard);
  // Who answered: for an admin, and for whoever shared the event - until
  // a guest list takes its place.
  const answered = el('div', 'detail-answered');
  if (!e.cancelled && (isAdmin() || postedAndHosting(e))) {
    answered.append(rsvpsCard(e));
  }
  side.append(answered);
  // Where the event came from, last.
  // Where the event came from - not for an event another app runs, whose
  // badge on the banner says it, unless an admin's own lines are due.
  const linked = e.source === 'celebrate' || e.source === 'team';
  const source = !linked || isAdmin() ? sourceCard(e, linked) : null;
  if (source) {
    side.append(source);
  }
  cols.append(side);
  page.append(cols);
  fillInvites(e, ask, answered, {info: when, linkedLine: page.querySelector('.hero-linked-line span')});
  return page;
}

// fillInvites fetches the event's guest list and draws what the viewer
// may see of it: the family band in place of the plain ask for someone
// invited, the guest list section for a host, and who is coming in the
// rail - or, for a host with no list yet, the way to start one.
async function fillInvites(e, ask, answered, {info, linkedLine}) {
  // Every hand-added or linked event asks, whoever is viewing: its Hosts
  // card is always drawn, even when nobody hosts it.
  if (e.cancelled || (e.source !== 'sheet' && !e.link && !imported(e))) {
    return;
  }
  let view = await fetchInvites(e);
  if (!view || !ask.isConnected) {
    return;
  }
  const refresh = async () => {
    const {load} = await import('../app.js');
    await load();
  };
  // Create Invite on Helios Celebrate or HCA-Team sends a host here with
  // ?invite=1: the list is started - the invitation, and a group for the
  // ticket holders or the volunteers that follows them - and the address
  // tidied.
  if (view.host && e.link && new URLSearchParams(location.search).get('invite') && !view.settings) {
    try {
      const made = await startParty(e);
      const who = isParty(e) ? ['ticket holders', 'the tickets'] : ['volunteers', 'the sign-ups'];
      toast(made.added ? `Guest list started with ${made.added} ${who[0]} - it follows ${who[1]} from here. Send the invites when it is ready.` : `Guest list started - it follows ${who[1]} from here.`, 6000);
    } catch (err) {
      toast(err.message);
    }
    history.replaceState(null, '', location.pathname);
    view = await fetchInvites(e);
    if (!view || !ask.isConnected) {
      return;
    }
  }
  // The ask heads the page until the family has answered in full; My
  // RSVP in the rail's card carries the answers throughout.
  if (view.mine.length) {
    // My RSVP in the rail's card carries an invited household's answers,
    // so the plain answer's card goes with the plain ask.
    ask.closest('.event-page')?.querySelector('.rsvp-compact')?.remove();
    ask.replaceChildren(familyAnswered(view) ? '' : familyBand(e, view, refresh));
  }
  // The invitation's tab joins Edit for a host; a linked event's host,
  // with no event form of their own, gets Edit for the invitation alone.
  editorView = view;
  // A linked event's host - a party's, an HCA event's chair - edits the
  // invitation from the start: its own words for the event, the email's
  // text, the flyer, who may see who is coming.
  if (view.host) {
    const tools = ask.closest('.event-page')?.querySelector('.detail-tools');
    if (tools && !tools.querySelector('.detail-edit')) {
      tools.append(button('Edit', 'pencil', 'button button-secondary button-small detail-edit', () => openEditor(e, view, refresh, {tab: 'invitation'})));
    }
  }
  // Who's coming takes the page's width under the ask, as Celebrate lays
  // out its party; a host's Guest list - the tools and every row - is
  // the rail's, where who answered used to be.
  // An event another app runs - a party, an HCA event - has nobody coming
  // here until someone is invited from its page, so it says nothing of it
  // until then.
  const invited = (view.host ? view.list : view.coming || []).some(r => r.invited);
  if (view.coming && (!e.link || invited)) {
    ask.append(comingCard(e, view, refresh));
  }
  if (view.host) {
    if (view.settings || view.list.some(r => r.invited)) {
      answered.replaceChildren(guestListSection(e, view, refresh));
    } else {
      ask.append(inviteHostCall(e, view, refresh));
      answered.replaceChildren();
    }
  } else if (e.invitation) {
    // A guest list takes the place of who answered; with none, an
    // admin's answers card stays.
    answered.replaceChildren();
  }
  // The rail's card gains Hosts, and My RSVP for anyone on the list; the
  // flyer sits right under the linked app's card. Hosts show whenever
  // anyone hosts - a host adds co-hosts and steps down there - unless the
  // hosts hid them, when the card is theirs alone.
  // A host - an admin on a school event among them - sees it even with
  // nobody listed, to add co-hosts.
  if ((view.hosts.length || view.host) && (view.host || !view.hostsHidden)) {
    info.append(hostsRow(e, view, refresh));
  }
  if (view.mine.length) {
    info.append(rsvpRow(e, view, refresh));
  }
  // The flyer follows the rail's card; the banner's badge names the hosts.
  const flyer = flyerCard(e, view, refresh);
  if (flyer) {
    info.after(flyer);
  }
  // Without one, a small Add a flyer at the rail's foot for a host.
  const addFlyer = addFlyerLink(e, view, refresh);
  if (addFlyer) {
    info.closest('.detail-side').append(addFlyer);
  }
  const hosts = view.hosts.map(h => h.name).filter(Boolean);
  if (linkedLine && hosts.length) {
    linkedLine.textContent = (isParty(e) ? 'Hosted by ' : 'Chaired by ') + (hosts.length > 1 ? hosts.slice(0, -1).join(', ') + ' and ' + hosts[hosts.length - 1] : hosts[0]);
  }
}

const aboutWords = {
  celebrate: 'Hosted by families in the community. The party page has the hosts, the price, and who is coming.',
  team: 'Run by the Helios Community Association. The event page has the roles to fill, who runs it, and who has signed up.',
};

// stampWords is a sheet timestamp, "2026-08-01 09:00" or a bare date, as
// "Aug 1, 2026".
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

// rsvpCard is the ask under the event, behind a ticked calendar: are you
// going? Yes, Maybe and No, the one given filled, and under them small
// links: Clear my RSVP once one is given, and Hide event (Show event once
// hidden) - a yes brings a calendar invite by email. A party has no yes
// or no: Add to my calendar with a ticket in the household, Add Ticket
// without one. Hiding is the cross on Heliosian's cards.
//
// Once a yes, a maybe or a no is given the band steps aside for a quieter
// card in the rail under the hosts, slot: My RSVP with the answer behind its
// mark, and under it Change response - which opens the three answers
// there, small, with Clear my RSVP - and Hide event; clearing the answer,
// or hiding the event, brings the band back.
function rsvpCard(e, slot) {
  const band = el('div', 'rsvp-band');
  // changing opens the rail's card to the three answers.
  let changing = false;
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
    // Under the buttons: an answer given can be taken back, and the event
    // hidden from the viewer's lists - or shown again once it is.
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
      button('Yes', 'check', 'button rsvp-yes' + (word === 'yes' ? ' is-on' : ''), () => say(word === 'yes' ? '' : 'yes')),
      button('Maybe', 'clock', 'button rsvp-maybe' + (word === 'maybe' ? ' is-on' : ''), () => say(word === 'maybe' ? '' : 'maybe')),
      button('No', 'close', 'button rsvp-no' + (word === 'no' ? ' is-on' : ''), () => say(word === 'no' ? '' : 'no')),
    ];
    if (settled) {
      const row = el('div', 'side-row');
      const icon = el('div', 'side-icon');
      icon.append(svg('calcheck'));
      const body = el('div', 'side-row-body');
      body.append(el('div', 'side-title', 'My RSVP'));
      if (changing) {
        // Changing: the three answers, small, the one given filled, with
        // Clear and a way back to the answer as it stands.
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
        // The answer as it stands - a mark and the words, as an invited
        // household's My RSVP reads - and quietly under it, Change
        // response and Hide event.
        const said = el('div', 'invite-said rsvp-compact-said is-' + word);
        const mark = el('span', 'invite-said-mark');
        mark.append(svg(word === 'yes' ? 'check' : word === 'maybe' ? 'clock' : 'close'));
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
        add.href = linkURL(e);
        add.append(svg('ticket'), el('span', '', e.availability === 'available' ? 'Add Ticket' : e.call || 'See the party'));
        buttons.append(add);
      }
    } else {
      // A yes, a maybe or a no is the rail's card, above; here the event
      // is unanswered or hidden.
      words.append(el('div', 'rsvp-title', word === 'hidden' ? 'Hidden' : 'Are you going?'), el('div', 'rsvp-lead', word === 'hidden' ? 'On the month in gray.' : 'Yes sends you a calendar invite.'));
      buttons.append(...answers());
    }
    if (note) {
      words.append(el('div', 'rsvp-note', note));
    }
    const side = el('div', 'rsvp-side');
    side.append(buttons, links);
    const row = el('div', 'rsvp-band-row');
    row.append(art, words, side);
    band.append(row);
  };
  paint();
  return band;
}

// inviteBand says an event is private - found by invitation or by its
// link alone - and offers the link to copy: whoever answers it has it on
// their calendar.
function inviteBand(e) {
  const band = el('div', 'pending-band is-invite');
  const words = el('div', 'pending-words');
  const mine = postedAndHosting(e);
  if (e.sharing === 'Invite Only') {
    words.append(el('div', 'pending-title', 'Invite only'), el('div', 'pending-lead', mine ? 'Only the people you invite can open this event, and their answers put it on their calendars.' : 'You were invited. Your answer below puts it on your calendar.'));
  } else {
    words.append(el('div', 'pending-title', 'Anyone with the link'), el('div', 'pending-lead', mine ? 'On the calendar of the people you invite, and of anyone you send this link to who answers.' : 'You were invited, or sent this link. Your answer below puts it on your calendar.'));
  }
  band.append(svg('link'), words);
  const url = location.origin + eventPath(e);
  band.append(button('Copy link', 'copy', 'button button-small', () => copyText(url, 'Link copied')));
  return band;
}

// pendingBand says a shared event is waiting for an admin - and, to an
// admin, offers to approve it onto the calendar or decline it away.
function pendingBand(e) {
  const band = el('div', 'pending-band' + (e.declined ? ' is-declined' : ''));
  const words = el('div', 'pending-words');
  const who = state.model.names && state.model.names[e.addedBy] ? state.model.names[e.addedBy] : e.addedBy;
  const mine = e.addedBy === state.model.user.email;
  // Only an admin with the hat on is asked to decide; anyone else reads
  // where it stands.
  // Deciding on a waiting event is any admin's, hat or not - an alert
  // for them; bringing back a declined one wants the hat.
  const decides = (e.pending && !e.declined ? isSystemAdmin() : isAdmin()) && !mine;
  if (e.declined) {
    words.append(el('div', 'pending-title', 'Declined'), el('div', 'pending-lead', mine ? 'An admin declined this event, so it is not on the calendar. You can still edit it; an admin can approve it later.' : decides ? `Shared by ${who} and declined. Approve it to put it on the calendar after all.` : `Shared by ${who}. An admin declined it, so it is not on the calendar.`));
  } else {
    words.append(el('div', 'pending-title', 'Waiting for approval'), el('div', 'pending-lead', mine ? 'You shared this event. An admin will approve it onto the calendar; until then it is shared by link, so anyone you send the link to can open it.' : decides ? `Shared by ${who}. Approve it onto the calendar, or decline it.` : `Shared by ${who}. It goes on the calendar once an admin approves it.`));
  }
  band.append(svg(e.declined ? 'close' : 'clock'), words);
  if (e.pending && !e.declined ? isSystemAdmin() : isAdmin()) {
    const actions = el('div', 'pending-actions');
    const decide = async (path, done) => {
      const res = await fetch('/api/calendar/events/' + path, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: e.id})});
      if (!res.ok) {
        toast(await res.text());
        return;
      }
      toast(done);
      const {load} = await import('../app.js');
      await load();
    };
    actions.append(button('Approve', 'check', 'button button-small', () => decide('approve', 'On the calendar')));
    if (!e.declined) {
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

// rsvpsCard is who answered, for an admin: the yeses and the nos as little
// contact cards - photo or initial, name, address - with counts. Who hid
// the event is nobody's business but theirs.
function rsvpsCard(e) {
  const card = el('div', 'side-card rsvps-card');
  const r = (state.model.responses || {})[e.id] || {};
  card.append(el('div', 'side-title', 'RSVPs'));
  const yes = r.yes || [];
  const no = r.no || [];
  if (!yes.length && !no.length && !(r.maybe || []).length) {
    card.append(el('div', 'side-line', 'Nobody has answered yet.'));
    return card;
  }
  const maybe = r.maybe || [];
  for (const [label, people] of [['Yes', yes], ['Maybe', maybe], ['No', no]]) {
    if (!people.length) {
      continue;
    }
    card.append(el('div', 'rsvps-head', `${label} · ${people.length}`));
    // Each is a face tile as HCA-Team draws its volunteers: the face, a
    // student's grade badged on its corner in Who?'s colour, the name
    // under it - opening their page on Who?.
    const list = el('div', 'rsvps-grid');
    for (const p of people) {
      const tile = el('a', 'contact-card');
      tile.href = appOrigin('who') + '/people/' + encodeURIComponent(p.email);
      tile.title = [p.name, p.line].filter(Boolean).join(' · ');
      const face = avatar(p, 'contact-photo');
      if (p.grade) {
        const badge = el('span', 'grade-badge', /^kindergarten$/i.test(p.grade) ? 'K' : p.grade.replace(/^grade\s*/i, ''));
        badge.title = p.grade;
        const color = (state.model.gradeColors || {})[p.grade];
        if (color) {
          badge.style.background = `color-mix(in srgb, ${color} 65%, black)`;
        }
        face.append(badge);
      }
      tile.append(face, el('span', 'contact-name', p.name || p.email));
      list.append(tile);
    }
    card.append(list);
  }
  return card;
}

// keywordsEditor is the search words an admin can change from the page:
// the words as chips, Edit making them a box, Save writing them to the
// event's Overrides row.
function keywordsEditor(e) {
  const wrap = el('div');
  const paint = () => {
    wrap.replaceChildren();
    const line = el('div', 'side-line', 'Search words' + (e.keywords && e.keywords.length ? ':' : ': none'));
    wrap.append(line);
    if (e.keywords && e.keywords.length) {
      const chips = el('div', 'side-keywords');
      for (const w of e.keywords) {
        chips.append(el('span', 'side-keyword', w));
      }
      wrap.append(chips);
    }
    const edit = button('Edit search words', 'pencil', 'link-button', () => {
      wrap.replaceChildren();
      const input = el('input', 'side-keywords-edit');
      input.type = 'text';
      input.value = (e.keywords || []).join(', ');
      input.placeholder = 'words, separated by commas';
      const save = button('Save', 'check', 'button button-small', async () => {
        const keywords = input.value.split(',').map(w => w.trim()).filter(Boolean);
        const res = await fetch('/api/calendar/keywords', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: e.id, keywords})});
        if (!res.ok) {
          toast(await res.text());
          return;
        }
        toast('Search words saved');
        const {load} = await import('../app.js');
        await load();
      });
      const cancel = button('Cancel', null, 'button button-secondary button-small', paint);
      const row = el('div', 'modal-actions');
      row.append(save, cancel);
      wrap.append(el('div', 'side-line', 'Search words:'), input, row);
      input.focus();
    });
    wrap.append(edit);
  };
  paint();
  return wrap;
}

// sourceCard says where the event's dates came from and links to the
// original: the feed's event in Google Calendar, the school's calendar page
// for a line of the year calendar, or who added it by hand and when. An HCA
// event the school also lists says so, with the way to HCA-Team's page. An
// admin also sees what the admins' tabs did: the classifier's filing and
// any correction, with its note - and for an event another app runs, whose
// badge on the banner says whose it is, that alone (adminOnly).
function sourceCard(e, adminOnly = false) {
  const card = el('div', 'side-card side-card-help');
  const row = el('div', 'side-row');
  const icon = el('div', 'side-icon');
  icon.append(svg('info'));
  const body = el('div', 'side-row-body');
  if (!adminOnly) {
    body.append(el('div', 'side-title', sourceWords(e)));
  }
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
    default:
      lines.push(aboutWords[e.source] || '');
  }
  for (const words of adminOnly ? [] : lines.filter(Boolean)) {
    body.append(el('div', 'side-line', words));
  }
  if (adminOnly) {
    // The badge on the banner has said whose the event is.
  } else if (e.source === 'pdf') {
    body.append(outLink(e.sourceUrl, 'Open the school\u2019s calendar page'));
  } else if (e.source === 'google' && e.sourceUrl) {
    body.append(outLink(e.sourceUrl, 'Open in Google Calendar'));
  } else if (e.source === 'sheet' && e.sourceUrl) {
    // The proof behind a hand-added event, named by where it lives; the
    // Veracross portal wants a parent login.
    const host = new URL(e.sourceUrl).hostname.replace(/^www\./, '');
    body.append(outLink(e.sourceUrl, host.includes('veracross') ? 'Open the source on Veracross' : `Open the source (${host})`));
    if (host.includes('veracross')) {
      body.append(el('div', 'side-line', 'Veracross asks for your parent portal login.'));
    }
  } else if (e.link && (e.source === 'celebrate' || e.source === 'team')) {
    body.append(outLink(linkURL(e), isParty(e) ? 'Open on Helios Celebrate' : 'Open on HCA-Team'));
  }
  // The school's listing of an HCA event carries HCA-Team's link too.
  if (!adminOnly && e.link && e.source !== 'celebrate' && e.source !== 'team') {
    body.append(el('div', 'side-line', 'Also listed on HCA-Team, which runs it.'), outLink(linkURL(e), 'Open on HCA-Team'));
  }
  if (isAdmin()) {
    const p = (state.model.provenance || {})[e.id] || {};
    const admin = el('div', 'side-admin' + (adminOnly ? ' is-alone' : ''));
    admin.append(el('div', 'side-admin-title', 'For admins'));
    if (p.enriched) {
      admin.append(el('div', 'side-line', `Filed under its tags by Claude${p.model ? ' (' + p.model + ')' : ''} on ${stampWords(p.enriched)}.`));
    }
    if (p.corrected && p.corrected.length) {
      admin.append(el('div', 'side-line', `Corrected in Overrides: ${p.corrected.join(', ').toLowerCase()}.${p.note ? ' Note: \u201c' + p.note + '\u201d' : ''}`));
    }
    if (!e.link || e.source === 'google' || e.source === 'pdf' || e.source === 'sheet') {
      admin.append(keywordsEditor(e));
    }
    if (adminOnly && admin.childElementCount === 1) {
      // Nothing to say beyond the title: no card.
      return null;
    }
    body.append(admin);
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

// linkedBadge is the card at the banner's foot for an event another app
// runs: that app's mark, what the event is there - a Fun(d)raiser party,
// an HCA volunteer event - and a line under it: who hosts it once the
// page knows (fillInvites fills it in), until then the household's own
// standing or where the tickets or sign-ups stand. The whole card is the
// way to the event's page on that app.
function linkedBadge(e) {
  const kind = isParty(e) ? 'celebrate' : 'team';
  const card = el('a', 'hero-linked' + (e.mine ? ' is-mine' : ''));
  card.href = linkURL(e);
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

// linkedLineWords is the badge's line before the hosts are known: the
// household's part, each by name, or where the tickets or sign-ups stand.
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
