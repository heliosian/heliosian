import {el} from '/elements.js';
import {andList} from '/andlist.js';
import {themed, wrapped, wordless, sealMark, kickerWords} from '/themes.js';
function shell(look, className) {
  const env = wrapped(el('div', `envelope ${className || ''}`), look);
  env.append(el('div', 'env-back'), el('div', 'env-peek'), el('div', 'env-pocket'));
  const flap = el('div', 'env-flap');
  flap.append(el('div', 'env-flap-out'), el('div', 'env-flap-in'));
  env.append(flap);
  return env;
}

export function miniEnvelope(look, name) {
  const env = shell(look, 'envelope-mini');
  env.append(el('span', 'env-seal', sealMark(look, name)));
  return env;
}

export function sealedEnvelope(look, name, to, onOpen) {
  const stage = themed(el('div', 'envelope-stage'), look);
  const env = shell(look);
  const peek = themed(env.querySelector('.env-peek'), look);
  peek.append(el('span', 'env-peek-kicker', kickerWords(look)), el('span', 'env-peek-title', name));
  const seal = el('button', 'env-seal', sealMark(look, name));
  seal.type = 'button';
  seal.setAttribute('aria-label', 'Open the invitation');
  env.append(seal);
  const address = el('p', 'env-address');
  address.append(el('span', 'env-to', to), el('span', 'env-hint', 'Tap the seal to open'));
  stage.append(env, address);
  const open = () => {
    if (stage.classList.contains('is-opening')) {
      return;
    }
    stage.classList.add('is-opening');
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) {
      onOpen();
      return;
    }
    setTimeout(() => onOpen(peek.getBoundingClientRect()), 1650);
  };
  seal.addEventListener('click', open);
  env.addEventListener('click', open);
  return stage;
}

export function hostLine(hostedBy, hosts) {
  return hostedBy || andList(hosts);
}

export function inviteCard(words) {
  const card = themed(el('article', 'invite-card'), words.look);
  const art = el('div', 'card-art');
  art.setAttribute('aria-hidden', 'true');
  const body = el('div', 'card-body');
  if (words.cancelled) {
    body.append(el('p', 'card-cancelled', 'This event has been cancelled'));
  }
  body.append(el('p', 'card-kicker', kickerWords(words.look)), el('h1', 'card-title', words.name));
  if (words.subtitle) {
    body.append(el('p', 'card-subtitle', words.subtitle));
  }
  body.append(el('div', 'card-rule'));
  body.append(el('p', 'card-day', words.day));
  if (words.hours) {
    body.append(el('p', 'card-hours', words.hours));
  }
  if (words.place || words.location) {
    const place = el('p', 'card-place');
    if (words.place) {
      place.append(el('span', 'card-place-name', words.place));
    }
    if (words.location) {
      place.append(el('span', 'card-place-address', words.location));
    }
    body.append(place);
  }
  if (words.description) {
    body.append(el('p', 'card-message', words.description));
  }
  const hosts = hostLine(words.hostedBy, words.hosts);
  if (hosts) {
    body.append(el('p', 'card-hosts', 'Hosted by ' + hosts));
  }
  if (!wordless(words.look)) {
    card.append(art, body);
    return card;
  }
  card.classList.add('is-wordless');
  const img = el('img', 'card-picture');
  img.src = words.look.cardPicture;
  img.alt = '';
  body.classList.add('card-words-hidden');
  card.append(img, body);
  return card;
}

export function whereWords(w) {
  return [w.place, w.location].filter(Boolean).join(', ');
}

export function mapQuery(w) {
  return w.location || w.place;
}
