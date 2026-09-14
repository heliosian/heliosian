import {state} from './state.js';
import {el, svg, button, toast, segmented} from './dom.js';

// The picture tools the admins' category images and everyone's shared
// events use: an upload, a search of the picture libraries, and the
// control that offers both beside a thumbnail.

// uploadImage sends a picked file up and answers with the name the sheet
// records and where the page can fetch it.
export async function uploadImage(file) {
  const body = new FormData();
  body.append('image', file);
  const res = await fetch('/api/calendar/image', {method: 'POST', body});
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

// openSheet is a layer over the page for something the tool needs mid-edit
// - the picture search - closing on its cross, Escape, or a click outside.
export function openSheet(title, node) {
  const layer = el('div', 'modal-overlay');
  const box = el('div', 'modal modal-wide');
  const header = el('div', 'modal-header');
  header.append(el('h2', '', title));
  const close = el('button', 'modal-close', '×');
  close.type = 'button';
  close.setAttribute('aria-label', 'Close');
  const shut = () => {
    layer.remove();
    document.removeEventListener('keydown', onKey, true);
  };
  const onKey = e => {
    if (e.key === 'Escape') {
      e.stopImmediatePropagation();
      shut();
    }
  };
  close.addEventListener('click', shut);
  layer.addEventListener('click', e => {
    if (e.target === layer) {
      shut();
    }
  });
  document.addEventListener('keydown', onKey, true);
  header.append(close);
  box.append(header, node);
  layer.append(box);
  document.body.append(layer);
  return shut;
}

function imageSources() {
  return (state.model && state.model.imageSources) || ['Wikimedia Commons'];
}

const sourceNotes = {
  'Unsplash': 'Free to use under the Unsplash License; the photographer is credited on each tile.',
  'Pexels': 'Free to use under the Pexels License; the photographer is credited on each tile.',
  'Pixabay': 'Photos, illustrations and vectors, free to use under the Pixabay Content License.',
  'Wikimedia Commons': 'Everything here is free to use; the licence is on each tile, and CC BY ones ask to be credited.',
  'Google Images': 'Pick a picture you have the right to use - a school photo, a poster, a public-domain image.',
};

// openImageSearch is the picture picker the other apps have: a search box,
// the sources the server is set up for, a grid of results, and a click on
// one imports it through the server - fetched and stored like an upload -
// handing the stored name and its address to onPicked.
export function openImageSearch(initial, onPicked) {
  const wrap = el('div', 'image-search');
  const bar = el('div', 'image-search-bar');
  const input = el('input');
  input.type = 'search';
  input.value = initial || '';
  const go = button('Search', 'search', 'button', () => run());
  const sources = imageSources();
  let source = sources[0];
  const status = el('div', 'image-search-status');
  const grid = el('div', 'image-search-grid');
  const note = el('div', 'admin-note', sourceNotes[source] || '');
  if (sources.length > 1) {
    wrap.append(segmented(sources.map(name => ({key: name, label: name})), source, picked => {
      source = picked;
      input.placeholder = `Search ${source}…`;
      note.textContent = sourceNotes[source] || '';
      for (const b of wrap.querySelectorAll('.segment')) {
        b.classList.toggle('is-on', b.textContent === source);
      }
      run();
    }));
  }
  input.placeholder = `Search ${source}…`;
  bar.append(input, go);
  wrap.append(bar, status, grid, note);
  let busy = false;
  const run = async () => {
    const q = input.value.trim();
    if (!q || busy) {
      return;
    }
    busy = true;
    status.textContent = 'Searching…';
    grid.replaceChildren();
    try {
      const res = await fetch(`/api/calendar/images/search?q=${encodeURIComponent(q)}&source=${encodeURIComponent(source)}`);
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const hits = await res.json();
      status.textContent = hits.length ? '' : 'Nothing found.';
      for (const hit of hits) {
        const tile = el('button', 'image-search-hit');
        tile.type = 'button';
        const img = el('img');
        img.src = hit.thumb;
        img.alt = hit.title;
        img.loading = 'lazy';
        img.addEventListener('load', () => img.classList.add('is-loaded'));
        tile.append(img, el('span', 'image-search-source', hit.credit || (hit.license ? `${hit.license} · ${hit.source}` : hit.source)));
        tile.title = `${hit.title} - ${hit.width}×${hit.height}${hit.license ? ` - ${hit.license}` : ''}`;
        tile.addEventListener('click', async () => {
          if (busy) {
            return;
          }
          busy = true;
          status.textContent = 'Importing…';
          tile.classList.add('is-picked');
          try {
            const imported = await fetch('/api/calendar/images/import', {
              method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({url: hit.url, download: hit.download || ''}),
            });
            if (!imported.ok) {
              throw new Error(await imported.text());
            }
            const {name} = await imported.json();
            shut();
            onPicked(name, '/' + name);
          } catch (err) {
            status.textContent = err.message;
            tile.classList.remove('is-picked');
          }
          busy = false;
        });
        grid.append(tile);
      }
    } catch (err) {
      status.textContent = err.message;
    }
    busy = false;
  };
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      e.preventDefault();
      run();
    }
  });
  const shut = openSheet('Find an image', wrap);
  input.focus();
  if (input.value) {
    run();
  }
}

// imageControl is a category's picture: a thumbnail that opens a file
// chooser, a search of the picture libraries, and a way to take the
// picture off. The picture is what an event under the tag wears when it
// has none of its own.
export function imageControl(t, onChange) {
  const wrap = el('span', 'admin-image');
  const pick = el('label', 'admin-image-pick');
  pick.title = t.imageUrl ? 'Change the image' : 'Set an image';
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  if (t.imageUrl) {
    const img = el('img');
    img.src = t.imageUrl;
    img.alt = '';
    pick.append(img);
  } else {
    pick.append(svg('image'));
  }
  pick.append(file);
  file.addEventListener('change', async () => {
    if (!file.files[0]) {
      return;
    }
    try {
      const made = await uploadImage(file.files[0]);
      t.image = made.name;
      t.imageUrl = made.url;
      onChange();
    } catch (err) {
      toast(err.message);
    }
  });
  wrap.append(pick);
  const find = button('', 'search', 'icon-button admin-image-find', () => openImageSearch(t.name, (name, url) => {
    t.image = name;
    t.imageUrl = url;
    onChange();
  }));
  find.setAttribute('aria-label', 'Find an image');
  find.title = 'Find an image';
  wrap.append(find);
  if (t.imageUrl) {
    const remove = button('', 'close', 'icon-button admin-image-remove', () => {
      t.image = '';
      t.imageUrl = '';
      onChange();
    });
    remove.setAttribute('aria-label', 'Remove the image');
    wrap.append(remove);
  }
  return wrap;
}

