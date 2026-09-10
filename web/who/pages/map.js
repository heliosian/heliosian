import {state, byEmail} from '../state.js';
import {el, svg, thumbUrl, withFrom} from '../dom.js';
import {familyLink, familySearchText} from '../families.js';
import {matchesFilters, familyMatchesFilters, filterControl} from '../filters.js';
import {resetMain} from '../chrome.js';

let mapsPromise = null;

function loadMaps() {
  if (!mapsPromise) {
    mapsPromise = new Promise(resolve => {
      window._mapsReady = resolve;
      const script = el('script');
      script.src = 'https://maps.googleapis.com/maps/api/js?key=' +
        encodeURIComponent(document.body.dataset.mapsKey) + '&callback=_mapsReady';
      script.async = true;
      document.head.append(script);
    });
  }
  return mapsPromise;
}

const pinIcon = 'data:image/svg+xml;charset=UTF-8,' + encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="34" height="34">' +
  '<path d="M20 10c0 4.993-5.539 10.193-7.399 11.799a1 1 0 0 1-1.202 0C9.539 20.193 4 14.993 4 10a8 8 0 0 1 16 0" fill="#244d53" stroke="#fff" stroke-width="1"/>' +
  '<circle cx="12" cy="10" r="3" fill="#fff"/></svg>');

function familyMapPopup(family) {
  const box = el('div', 'map-popup');
  if (family.photoUrl) {
    const img = el('img', 'map-popup-photo');
    img.src = thumbUrl(family.photoUrl);
    img.alt = '';
    box.append(img);
  }
  const body = el('div', 'map-popup-body');
  body.append(el('div', 'map-popup-name', family.name));
  if (family.address) {
    body.append(el('div', 'map-popup-sub', family.address));
  }
  const link = el('a', 'map-popup-link', 'See family');
  link.href = familyLink(family.key);
  body.append(link);
  box.append(body);
  return box;
}

// Shared by the standalone Map page and a tag list's own Map view: builds the
// map into canvas, showing only families for which familyMatches(family) is
// true, and returns a renderPins() to call again after a filter/search change
// without recreating the map itself.
export function initFamilyMap(canvas, familyMatches) {
  let map = null;
  let info = null;
  let markers = [];
  const missing = el('div', 'map-missing');
  canvas.after(missing);

  function renderPins() {
    if (!map) {
      return;
    }
    for (const m of markers) {
      m.setMap(null);
    }
    markers = [];
    const allPeople = new Map();
    const withoutAddress = new Map();
    for (const family of Object.values(state.model.families)) {
      if (!familyMatches(family)) {
        continue;
      }
      const emails = [...(family.kidEmails || []), ...(family.adultEmails || [])];
      const matchingMembers = emails.map(e => byEmail[e]).filter(p => p && matchesFilters(p));
      for (const p of matchingMembers) {
        allPeople.set(p.email, p);
      }
      if (!family.address || family.veracrossAddress === 'partial') {
        for (const p of matchingMembers) {
          withoutAddress.set(p.email, p);
        }
        continue;
      }
      if (!family.lat && !family.lng) {
        continue;
      }
      const marker = new google.maps.Marker({
        map,
        position: {lat: family.lat, lng: family.lng},
        icon: {url: pinIcon, anchor: new google.maps.Point(17, 33)},
        title: family.name,
      });
      marker.addListener('click', () => {
        info.setContent(familyMapPopup(family));
        info.open(map, marker);
      });
      markers.push(marker);
    }
    missing.replaceChildren();
    if (withoutAddress.size > 10) {
      missing.textContent = `${withoutAddress.size} of ${allPeople.size} people not shown — no street address on file`;
    } else if (withoutAddress.size) {
      const names = [...withoutAddress.values()].map(p => p.fullName).sort((a, b) => a.localeCompare(b));
      missing.textContent = `Not shown, no street address on file: ${names.join(', ')}`;
    }
  }

  loadMaps().then(() => {
    if (!canvas.isConnected) {
      return;
    }
    map = new google.maps.Map(canvas, {
      mapTypeControl: false,
      streetViewControl: false,
      fullscreenControl: false,
    });
    info = new google.maps.InfoWindow({headerDisabled: true});
    map.addListener('click', () => info.close());
    const bounds = new google.maps.LatLngBounds();
    for (const family of Object.values(state.model.families)) {
      if ((family.lat || family.lng) && family.veracrossAddress !== 'partial' && familyMatches(family)) {
        bounds.extend({lat: family.lat, lng: family.lng});
      }
    }
    map.fitBounds(bounds);
    renderPins();
  });

  return () => renderPins();
}

export function renderMapPage() {
  const main = resetMain();

  const content = el('div', 'content container');
  const header = el('div', 'content-header');
  header.append(el('h1', '', 'Map'));
  const controls = el('div', 'controls');
  const search = el('div', 'search');
  search.append(svg('search'));
  const input = el('input');
  input.placeholder = 'Search';
  input.value = state.q;
  let renderPins = () => {};
  input.addEventListener('input', () => {
    state.q = input.value.trim().toLowerCase();
    renderPins();
  });
  search.append(input);
  controls.append(search, filterControl(() => renderPins()));
  header.append(controls);
  content.append(header);

  const canvas = el('div', 'map-canvas');
  content.append(canvas);

  const update = el('div', 'map-update');
  const action = el('a', 'map-update-link');
  action.href = withFrom('/my-privacy');
  action.append(svg('zap'), el('span', '', 'Update My Address'));
  update.append(action);
  content.append(update);
  main.append(content);

  renderPins = initFamilyMap(canvas, family => familyMatchesFilters(family.key) && familySearchText(family).includes(state.q));
}
