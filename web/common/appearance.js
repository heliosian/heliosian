// The Appearance card of every app's admin tools: the page's two colours,
// the rail's and the page's, each a swatch beside its hex, with a second
// pair that makes it a gradient running down from the first to the
// second; the colour of the rail's words, one swatch; and two pictures,
// the rail's logo and the art at its foot, each uploaded to the app's
// picture route and named in the theme. Blank means the stylesheet's own, which the swatch shows so the
// picker starts from it; picking a first colour fills an empty second with
// it, so a colour is solid until a second is chosen, and clearing the
// first clears both. A change to any posts them all to the app's theme
// route (internal/theme) at once.
//
// defaults are the app's own colours, for the swatches and the
// placeholders: {sidebar, sidebarEnd, sidebarText, page, pageEnd}.

const keys = ['sidebar', 'sidebarEnd', 'sidebarText', 'page', 'pageEnd', 'logo', 'sidebarImage'];
const hex = /^#[0-9a-f]{6}$/i;

const css = `
.appearance-row { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; padding: 12px 0; border-top: 1px solid var(--line); }
.appearance-row:first-of-type { border-top: none; }
.appearance-row .appearance-words { flex: 1 1 200px; min-width: 0; }
.appearance-row .appearance-title { font-weight: 600; }
.appearance-row .appearance-meta { font-size: 13px; color: var(--muted); }
.appearance-fields { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.appearance-field { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; color: var(--muted); }
.appearance-field input[type="color"] { width: 40px; height: 32px; padding: 2px; border: 1px solid var(--line); border-radius: 8px; background: #fff; cursor: pointer; }
.appearance-field input[type="text"] { width: 90px; padding: 6px 9px; border: 1px solid var(--line); border-radius: 8px; font: inherit; font-size: 13.5px; font-family: ui-monospace, Menlo, monospace; color: var(--ink); }
.appearance-reset { padding: 0; border: 0; background: none; color: var(--brand); font: inherit; font-size: 13px; text-decoration: underline; cursor: pointer; }
.appearance-picture { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.appearance-preview { display: flex; align-items: center; justify-content: center; width: 120px; height: 64px; border: 1px solid var(--line); border-radius: 8px; background: #fff; overflow: hidden; }
.appearance-preview.is-dark { background: #2e5c63; }
.appearance-preview img { max-width: 100%; max-height: 100%; object-fit: contain; }
.appearance-preview span { font-size: 12px; color: var(--muted); }
.appearance-upload { display: inline-flex; align-items: center; padding: 6px 14px; border: 1px solid var(--line); border-radius: 999px; background: #fff; color: var(--brand); font: inherit; font-size: 13px; font-weight: 600; cursor: pointer; }
.appearance-upload:hover { border-color: var(--brand); }
.appearance-upload input { display: none; }
.appearance-status { margin-top: 10px; font-size: 13px; color: var(--muted); }
.appearance-status.is-error { color: var(--alert, #c0392b); }
`;

function ensureStyle() {
  if (!document.querySelector('#appearance-style')) {
    const style = document.createElement('style');
    style.id = 'appearance-style';
    style.textContent = css;
    document.head.append(style);
  }
}

function make(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

// pictures are the app's own logo and rail art, for the previews while
// none is uploaded: {logo, sidebarImage}, each a path or blank; darkRail
// says the rail is dark, so the previews sit on a dark ground.
export function appearanceCard({theme, url, defaults, sidebarWords, pageWords, pictures, darkRail}) {
  ensureStyle();
  const current = {};
  for (const key of keys) {
    current[key] = (theme && theme[key]) || '';
  }
  const card = make('div', 'card');
  card.append(make('h2', '', 'Appearance'));
  card.append(make('div', 'hint', 'The page’s colours and pictures: the rail down the left, the words on it, the page behind everything else, and the logo and picture on the rail. Pick a colour or type its hex; give it a second colour to run down to for a gradient, or leave that matching for a solid. Use default puts one back. Changes save immediately and reach everyone on their next load.'));
  const rows = make('div');
  const status = make('div', 'appearance-status');
  card.append(rows, status);

  async function persist() {
    status.classList.remove('is-error');
    status.textContent = 'Saving…';
    const res = await fetch(url, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(current),
    });
    if (!res.ok) {
      status.classList.add('is-error');
      status.textContent = await res.text();
      return;
    }
    status.textContent = 'Saved.';
  }

  // shows are how every field is redrawn after a change, since the end
  // colour follows the start.
  const shows = [];
  async function set(key, value) {
    const was = current[key];
    current[key] = value.toLowerCase();
    for (const [start, end] of [['sidebar', 'sidebarEnd'], ['page', 'pageEnd']]) {
      if (key === start && (!current[end] || current[end] === was)) {
        current[end] = current[start];
      }
      if (!current[start]) {
        current[end] = '';
      }
    }
    shows.forEach(f => f());
    await persist();
  }

  function field(key, label) {
    const wrap = make('label', 'appearance-field', label);
    const swatch = make('input');
    swatch.type = 'color';
    swatch.setAttribute('aria-label', label + ' colour');
    const text = make('input');
    text.type = 'text';
    text.placeholder = defaults[key];
    text.maxLength = 7;
    text.setAttribute('aria-label', label + ' colour, as hex');
    const show = () => {
      swatch.value = current[key] || defaults[key];
      text.value = current[key];
    };
    shows.push(show);
    swatch.addEventListener('change', () => set(key, swatch.value));
    text.addEventListener('change', () => {
      const value = text.value.trim();
      if (value === '' || hex.test(value)) {
        set(key, value);
        return;
      }
      status.classList.add('is-error');
      status.textContent = 'A colour is six hex digits after a #, like #2e5c63.';
      show();
    });
    wrap.append(swatch, text);
    show();
    return wrap;
  }

  // The two backgrounds take a gradient's pair of colours; the rail's
  // words take one.
  for (const [key, title, meta, single] of [
    ['sidebar', 'Sidebar', sidebarWords || 'The rail down the left'],
    ['sidebarText', 'Sidebar text', 'The words and icons on the rail', true],
    ['page', 'Main screen', pageWords || 'The page behind everything else'],
  ]) {
    const row = make('div', 'appearance-row');
    const words = make('div', 'appearance-words');
    words.append(make('div', 'appearance-title', title), make('div', 'appearance-meta', meta));
    const fields = make('div', 'appearance-fields');
    const reset = make('button', 'appearance-reset', 'Use default');
    reset.type = 'button';
    reset.addEventListener('click', () => set(key, ''));
    if (single) {
      fields.append(field(key, 'Colour'), reset);
    } else {
      fields.append(field(key, 'From'), field(key + 'End', 'Down to'), reset);
    }
    row.append(words, fields);
    rows.append(row);
  }

  // A picture: what is there now, a way to send up another, and the way
  // back to the app's own. The upload answers the stored name, which then
  // goes into the theme like a colour.
  function picture(key, label) {
    const wrap = make('div', 'appearance-picture');
    const preview = make('div', 'appearance-preview' + (darkRail ? ' is-dark' : ''));
    const show = () => {
      preview.replaceChildren();
      const src = current[key] ? '/' + current[key] : (pictures && pictures[key]) || '';
      if (src) {
        const img = make('img');
        img.src = src;
        img.alt = '';
        preview.append(img);
      } else {
        preview.append(make('span', '', 'None'));
      }
    };
    shows.push(show);
    const upload = make('label', 'appearance-upload', 'Upload…');
    const file = make('input');
    file.type = 'file';
    file.accept = 'image/png,image/jpeg,image/gif,image/webp';
    file.setAttribute('aria-label', 'Upload a ' + label);
    file.addEventListener('change', async () => {
      if (!file.files.length) {
        return;
      }
      const body = new FormData();
      body.append('image', file.files[0]);
      file.value = '';
      status.classList.remove('is-error');
      status.textContent = 'Uploading…';
      const res = await fetch(url + '/picture', {method: 'POST', body});
      if (!res.ok) {
        status.classList.add('is-error');
        status.textContent = await res.text();
        return;
      }
      const {name} = await res.json();
      current[key] = name;
      shows.forEach(f => f());
      await persist();
    });
    upload.append(file);
    const reset = make('button', 'appearance-reset', 'Use default');
    reset.type = 'button';
    reset.addEventListener('click', async () => {
      current[key] = '';
      shows.forEach(f => f());
      await persist();
    });
    show();
    wrap.append(preview, upload, reset);
    return wrap;
  }
  for (const [key, title, meta] of [
    ['logo', 'Logo', 'The lockup at the top of the rail - PNG with a clear background reads best'],
    ['sidebarImage', 'Sidebar bottom image', 'The picture at the foot of the rail, drawn across its width'],
  ]) {
    const row = make('div', 'appearance-row');
    const words = make('div', 'appearance-words');
    words.append(make('div', 'appearance-title', title), make('div', 'appearance-meta', meta));
    row.append(words, picture(key, title));
    rows.append(row);
  }
  return card;
}
