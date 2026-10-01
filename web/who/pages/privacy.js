import {privacyLinks, peopleOf, viewer} from '../state.js';
import {infoBanner} from '../dom.js';
import {el, svg} from '/elements.js';
import {familyOf} from '../families.js';

const veracrossAddressLabels = {full: 'Full Address', partial: 'Partial (City Only)', hidden: 'Hidden'};
const veracrossPhoneLabels = {visible: 'Visible', mixed: 'Mixed', hidden: 'Hidden'};

function privacyDotColor(state) {
  if (state === 'hidden') {
    return 'red';
  }
  return state === 'full' || state === 'visible' ? 'green' : 'yellow';
}

function privacyVeracrossCell(state, labels) {
  const cell = el('td', 'privacy-cell');
  const inner = el('span', 'privacy-cell-inner');
  inner.append(el('span', `privacy-dot privacy-dot-${privacyDotColor(state)}`), el('span', '', labels[state] || state));
  cell.append(inner);
  return cell;
}

function privacyHeliosCell(masked) {
  const cell = el('td', 'privacy-cell');
  const badge = el('span', `privacy-icon ${masked ? 'privacy-icon-lock' : 'privacy-icon-sync'}`);
  badge.append(svg(masked ? 'lock' : 'sync'));
  const inner = el('span', 'privacy-cell-inner');
  inner.append(badge, el('span', '', masked ? 'Hidden (Override Veracross)' : 'Matches Veracross'));
  cell.append(inner);
  return cell;
}

function familyShownPhone(family) {
  return peopleOf(family.adults)
    .filter(p => p.phone)
    .map(p => p.phone)
    .join(', ');
}

function privacyRow(label, veracrossState, veracrossLabels, masked, shownValue) {
  const tr = el('tr');
  tr.append(el('td', 'privacy-row-label', label));
  tr.append(privacyVeracrossCell(veracrossState, veracrossLabels));
  tr.append(privacyHeliosCell(masked));
  tr.append(el('td', 'privacy-cell privacy-shown', shownValue || '(Hidden)'));
  return tr;
}

function privacyWarningBanner(label) {
  return infoBanner(
    'alert', 'warn',
    `Your ${label} is visible on Veracross but hidden here`,
    "Hiding it in the Helios Who app does not hide it on Veracross - anyone with Veracross access can still see it there. " +
      "To match what Veracross already shows, sync your Helios Who opt-in.",
    'Sync Now', privacyLinks.heliosWhoOptIn, true);
}

function privacyOptinImage(src, alt) {
  const img = el('img', 'privacy-optin-image');
  img.src = src;
  img.alt = alt;
  img.loading = 'lazy';
  return img;
}

function privacyActionButton(iconName, label, href) {
  const a = el('a', 'media-button primary privacy-action');
  a.href = href;
  a.target = '_blank';
  a.rel = 'noopener';
  a.append(svg(iconName), el('span', '', label));
  return a;
}

export function privacyPage() {
  const page = document.createDocumentFragment();
  const family = familyOf(viewer());
  if (!family) {
    page.append(el('div', 'container', 'No family record found for your account.'));
    return page;
  }

  const warnings = privacyWarnings(family);
  for (const label of warnings) {
    page.append(privacyWarningBanner(label));
  }

  const content = el('div', 'content container privacy-page');
  content.append(el('h1', 'page-title', 'Your Privacy'));

  const intro = el('div', 'privacy-intro');
  intro.append(el('p', '',
    "Helios Who's data comes from Veracross, but you can further restrict what's shown here. This means:"));
  const list = el('ul', '');
  list.append(el('li', '', 'Hiding your phone or address here does not hide it on Veracross.'));
  list.append(el('li', '',
    "Matching your Helios Who visibility to Veracross will never show more here than Veracross already shows."));
  intro.append(list);
  content.append(intro);

  const holder = el('div', 'data-grid-wrap');
  const table = el('table', 'data-grid privacy-table');
  const thead = el('thead');
  const headRow = el('tr');
  for (const label of ['', 'Veracross', 'Helios Who', 'Shown Here']) {
    headRow.append(el('th', '', label));
  }
  thead.append(headRow);
  const tbody = el('tbody');
  tbody.append(privacyRow('Phone', family.veracrossPhone, veracrossPhoneLabels, family.phoneMasked, familyShownPhone(family)));
  tbody.append(privacyRow('Address', family.veracrossAddress, veracrossAddressLabels, family.addressMasked, family.address));
  table.append(thead, tbody);
  holder.append(table);
  content.append(holder);

  const actions = el('div', 'privacy-actions');
  actions.append(
    privacyActionButton('edit', 'Update Veracross', privacyLinks.veracrossPreferences),
    privacyActionButton('sync', 'Update Helios Who Visibility', privacyLinks.heliosWhoOptIn));
  content.append(actions);

  if (warnings.length > 0) {
    const howTo = el('div', 'privacy-howto');
    howTo.append(el('h2', 'privacy-howto-title', 'How to Match Veracross'));
    const syncNote = el('p', '');
    syncNote.append(
      'To share the same information as on Veracross, complete the ',
      (() => {
        const a = el('a', '', 'opt in form');
        a.href = privacyLinks.heliosWhoOptIn;
        a.target = '_blank';
        a.rel = 'noopener';
        return a;
      })(),
      ' and select both checkboxes. This will only share what is already on Veracross.');
    howTo.append(syncNote);

    const optinImages = el('div', 'privacy-optin-images');
    optinImages.append(
      privacyOptinImage('/help/optin-status.png', 'Communication Opt-In Status: choose "I agree to have family names and emails in the Helios Community Apps".'),
      privacyOptinImage('/help/optin-checkboxes.png', 'Opt-In Information: check both Home Address and Adult Phone Number.'));
    howTo.append(optinImages);
    const resolveActions = el('div', 'privacy-actions');
    resolveActions.append(privacyActionButton('sync', 'Resolve Now', privacyLinks.heliosWhoOptIn));
    howTo.append(resolveActions);
    content.append(howTo);
  }

  page.append(content);
  return page;
}

function privacyWarnings(family) {
  const warnings = [];
  if (family.addressMasked && family.veracrossAddress !== 'hidden') {
    warnings.push('address');
  }
  if (family.phoneMasked && family.veracrossPhone !== 'hidden') {
    warnings.push('phone number');
  }
  return warnings;
}

export function myPrivacyWarnings() {
  const family = familyOf(viewer());
  return family ? privacyWarnings(family) : [];
}

export function privacyMismatchCardDismissed() {
  return localStorage.getItem('privacyMismatchCardDismissed') === '1';
}

export function privacyMismatchText(warnings) {
  return `Your ${warnings.join(' and ')} ${warnings.length === 1 ? 'is' : 'are'} visible on ` +
    'Veracross but hidden in Helios Who. Hiding it here does not hide it on Veracross.';
}

export function privacyMismatchCard(warnings) {
  return infoBanner(
    'alert', 'warn', 'Privacy Settings Mismatch', privacyMismatchText(warnings),
    'See Details', '/my-privacy', false,
    () => {
      localStorage.setItem('privacyMismatchCardDismissed', '1');
    });
}
