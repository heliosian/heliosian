import {el, button} from '/elements.js';
import {setTitle, renderChrome, pageHead} from '/shell.js';
import {loadRow, load, navigate} from '/router.js';
import {appOrigin} from '/origins.js';
import {openSheet, fillDeck} from '/sheets.js';
import {fetchInvites as fetchGuestList} from '/guestlist.js';
import {guestListSection, inviteHostCall, comingCard} from '/invites.js';
import {findInvite, fetchInvite, me, loadPages, pagesOf, state} from '../state.js';
import {loadDetails, noteOpened, asEvent, rsvpPanel, guestListWords, deckOf, cardWords} from '/invitation.js';
import {openPageEditor} from '../pageeditor.js';
import {andList} from '/andlist.js';
import {sealedEnvelope, inviteCard} from '/envelope.js';
import {openEditor} from '../editor.js';
import {themed, sidebarIn} from '/themes.js';
import {hostQuestions} from '../questions.js';
import {notesPanel} from '/hostnotes.js';

const opened = new Set();

export function invitePage(key, section, pageId) {
  return loadRow(key, {find: findInvite, load: fetchInvite, show: inv => drawSection(inv, key, section, pageId), missing: 'That invitation'});
}

function drawSection(inv, key, section, pageId) {
  if (!inv.theme) {
    location.replace(appOrigin('when') + inv.path);
    return el('div', 'list-page');
  }
  sidebarIn(inv.look);
  loadPages(inv).then(renderChrome);
  if (section === 'guests') {
    return sectionPage(inv, guestListWords(inv), guestsBody(inv));
  }
  if (section === 'questions' && inv.host) {
    const body = el('div');
    loadDetails(inv).then(details => body.replaceChildren(hostQuestions(inv, details)));
    return sectionPage(inv, 'Questions', body);
  }
  if (section === 'notes' && inv.host) {
    return sectionPage(inv, 'Notes from guests', notesPanel(asEvent(inv), true));
  }
  if (section === 'p') {
    opened.add(inv.id);
  }
  return draw(inv, key, section === 'p' ? pageId : '');
}

function sectionPage(inv, title, body) {
  setTitle(`${title} · ${inv.name}`);
  const page = el('div', 'invite-section-page');
  body.classList.add('invite-section-body');
  page.append(pageHead(title), body);
  const backdrop = themed(el('div', 'invite-backdrop'), inv.look);
  backdrop.append(page);
  return backdrop;
}

function guestsBody(inv) {
  const body = el('div', 'host-guests');
  const e = asEvent(inv);
  const refresh = () => load();
  fetchGuestList(e).then(view => {
    if (!inv.host) {
      body.replaceChildren(comingCard(e, view, refresh));
      return;
    }
    body.replaceChildren(view.list.some(r => r.invited) ? guestListSection(e, view, refresh) : inviteHostCall(e, view, refresh));
  }, err => {
    body.replaceChildren(el('p', 'save-status error', 'Couldn’t load the guest list: ' + err.message));
  });
  return body;
}

function openPage(inv, key, p) {
  return openSheet(inv.look, p, ...(inv.host ? [button('Edit', 'edit', 'invite-sheet-edit', () => openPageEditor(inv, p, key))] : []));
}

function deck(inv, key, focus, above, below) {
  const base = `/g/${encodeURIComponent(key)}`;
  return fillDeck(inv.look, deckOf(inv, pagesOf(inv)), focus, above, below, item => navigate(item.card ? base : `${base}/p/${item.id}`));
}

function addressedTo(inv, details) {
  if (details.household.length) {
    const mine = details.household.some(p => p.id === me().id);
    const others = details.household.filter(p => p.id !== me().id).map(p => p.short);
    return 'For ' + andList(mine ? ['you', ...others] : others);
  }
  return inv.host ? 'How your guests will see it' : 'For you';
}

function showCard(inv, details, stage, from) {
  const card = inviteCard(cardWords(inv, details));
  if (!from) {
    stage.replaceChildren(card);
    return;
  }
  const old = stage.firstChild;
  old.classList.add('is-outgoing', 'is-leaving');
  stage.append(card);
  const to = card.getBoundingClientRect();
  card.style.transformOrigin = 'top left';
  card.style.transform = `translate(${from.left - to.left}px, ${from.top - to.top}px) scale(${from.width / to.width})`;
  card.getBoundingClientRect();
  card.style.transition = 'transform 0.8s cubic-bezier(0.2, 0.8, 0.2, 1)';
  card.style.transform = '';
  setTimeout(() => old.remove(), 600);
}

function draw(inv, key, focus) {
  setTitle(inv.name);
  const page = themed(el('div', 'invite-page'), inv.look);
  const column = el('div', 'invite-column');
  const stage = el('div', 'invite-stage');
  const above = el('div', 'invite-sheets is-above');
  const below = el('div', 'invite-sheets is-below');
  const side = el('div', 'invite-side');
  stage.append(el('div', 'invite-loading', 'Opening…'));
  column.append(above, stage, below);
  page.append(column, side);
  const fillSide = details => {
    side.replaceChildren();
    if (inv.host && !page.querySelector('.invite-host-tools')) {
      const tools = el('div', 'invite-host-tools');
      tools.append(
        button('Preview', 'eye', 'button button-secondary button-small', () => {
          location.href = '/preview/' + inv.id;
        }),
        button('Edit', 'edit', 'button button-secondary button-small invite-edit', () => openEditor(inv)),
      );
      page.append(tools);
    }
    if (details.household.length && inv.status !== 'cancelled') {
      const rsvp = rsvpPanel(inv, details, redraw);
      side.append(rsvp);
      if (!inv.host) {
        side.append(notesPanel(asEvent(inv), false));
      }
      if (state.showRsvp) {
        state.showRsvp = false;
        requestAnimationFrame(() => rsvp.scrollIntoView({behavior: 'smooth', block: 'start'}));
      }
    }
  };
  const redraw = async () => fillSide(await loadDetails(inv));
  loadDetails(inv).then(details => {
    if (opened.has(inv.id)) {
      loadPages(inv).then(() => {
        const item = deck(inv, key, focus, above, below);
        if (item.card) {
          showCard(inv, details, stage, false);
        } else {
          stage.replaceChildren(openPage(inv, key, item));
        }
        fillSide(details);
      });
      return;
    }
    page.classList.add('is-sealed');
    const sealed = sealedEnvelope(inv.look, inv.name, addressedTo(inv, details), from => {
      opened.add(inv.id);
      page.classList.remove('is-sealed');
      showCard(inv, details, stage, from);
      fillSide(details);
      setTimeout(() => loadPages(inv).then(() => deck(inv, key, '', above, below)), 900);
      noteOpened(inv, details);
    });
    stage.replaceChildren(sealed);
    setTimeout(() => {
      if (!sealed.isConnected) {
        return;
      }
      sealed.querySelector('.env-seal').click();
    }, 1500);
  });
  return page;
}
