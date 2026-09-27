import {openCropTool} from '/crop.js';
import {popup} from '/modal.js';
import {api} from '/api.js';
import {el, svg, toast} from '/elements.js';

export function imageTools(apiBase, {state}) {
  async function uploadImage(file) {
    const body = new FormData();
    body.append('image', file);
    const {name} = await api('POST', `${apiBase}/image`, body);
    return {name, url: '/' + name};
  }

  async function uploadAndSave(save, file) {
    try {
      const {name} = await uploadImage(file);
      await save({image: name});
    } catch (err) {
      toast(err.message);
    }
  }

  function imageSearchOn() {
    return Boolean(state.model && state.model.imageSearch);
  }

  function openImageSearch(initial, onPicked) {
    const wrap = el('div', 'image-search');
    const bar = el('div', 'image-search-bar');
    const input = el('input');
    input.type = 'search';
    input.value = initial || '';
    input.placeholder = 'Search for a picture…';
    const go = el('button', 'button');
    go.type = 'button';
    go.append(svg('search'), el('span', '', 'Search'));
    go.addEventListener('click', () => run());
    bar.append(input, go);
    const status = el('div', 'image-search-status');
    const grid = el('div', 'image-search-grid');
    wrap.append(bar, status, grid);
    let busy = false;
    const pick = async (hit, tile) => {
      if (busy) {
        return;
      }
      busy = true;
      status.textContent = 'Importing…';
      tile.classList.add('is-picked');
      try {
        const {name} = await api('POST', `${apiBase}/images/import`, {id: hit.id});
        shut();
        await onPicked({name, url: '/' + name});
      } catch (err) {
        status.textContent = err.message;
        tile.classList.remove('is-picked');
      }
      busy = false;
    };
    const run = async () => {
      const q = input.value.trim();
      if (!q || busy) {
        return;
      }
      busy = true;
      status.textContent = 'Searching…';
      grid.replaceChildren();
      try {
        const hits = await api('GET', `${apiBase}/images/search?q=${encodeURIComponent(q)}`);
        status.textContent = hits.length ? '' : 'Nothing found.';
        for (const hit of hits) {
          const tile = el('button', 'image-search-hit');
          tile.type = 'button';
          const img = el('img');
          img.src = hit.thumb;
          img.alt = hit.title;
          img.loading = 'lazy';
          img.addEventListener('load', () => img.classList.add('is-loaded'));
          tile.append(img);
          tile.title = `${hit.title} - ${hit.width}×${hit.height}`;
          tile.addEventListener('click', () => pick(hit, tile));
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
    const {shut} = popup('Find an image', wrap, {wide: true});
    input.focus();
    if (input.value) {
      run();
    }
  }

  function imagePicker(current, currentUrl, options = {}) {
    const dropzone = Boolean(options.dropzone);
    const plain = Boolean(options.plain);
    const wrap = el('div', 'field');
    wrap.append(el('span', '', options.label || 'Image'));
    if (options.hint) {
      wrap.append(el('small', 'field-lead', options.hint));
    }
    const row = el('div', dropzone ? 'image-drop' : 'image-row');
    const preview = el('img');
    preview.alt = '';
    const placeholder = el('div', 'image-placeholder');
    if (dropzone) {
      placeholder.append(svg('image'), el('strong', '', 'Drag and drop an image here'), el('small', '', 'or click to choose a file'));
    } else {
      placeholder.append(svg('image'), el('strong', '', 'No image'), el('small', '', 'JPG, PNG or GIF'));
    }
    const choose = el('label', 'button button-secondary button-small', dropzone ? 'Choose image' : 'Choose');
    const file = el('input');
    file.type = 'file';
    file.accept = 'image/*';
    file.hidden = true;
    choose.append(file);
    const remove = el('button', 'link-button', 'Remove');
    remove.type = 'button';
    const crop = el('button', 'link-button', 'Crop');
    crop.type = 'button';
    let name = current || '';
    const show = url => {
      preview.hidden = !url;
      placeholder.hidden = Boolean(url);
      remove.hidden = !url;
      crop.hidden = !url || plain;
      if (url) {
        preview.src = url;
      }
    };
    const setStatus = (message, error) => {
      const status = wrap.closest('form').querySelector('.save-status');
      status.textContent = message;
      status.classList.toggle('error', Boolean(error));
    };
    show(currentUrl);
    const upload = async picked => {
      if (!picked) {
        return;
      }
      setStatus('Uploading image…');
      try {
        const made = await uploadImage(picked);
        name = made.name;
        show(made.url);
        setStatus('');
      } catch (err) {
        setStatus(err.message, true);
      }
      file.value = '';
    };
    file.addEventListener('change', () => upload(file.files[0]));
    remove.addEventListener('click', () => {
      name = '';
      show('');
    });
    crop.addEventListener('click', () => openCropTool(preview.src, false, async blob => {
      await upload(new File([blob], 'crop.jpg', {type: 'image/jpeg'}));
      return true;
    }));
    const find = el('button', 'button button-secondary button-small image-find', 'Find an image');
    find.type = 'button';
    find.hidden = plain || !imageSearchOn();
    find.addEventListener('click', e => {
      e.stopPropagation();
      openImageSearch(options.query ? options.query() : '', picked => {
        name = picked.name;
        show(picked.url);
      });
    });
    if (dropzone) {
      row.addEventListener('click', e => {
        if (!e.target.closest('label, button')) {
          file.click();
        }
      });
      row.addEventListener('dragover', e => {
        e.preventDefault();
        row.classList.add('is-dragover');
      });
      row.addEventListener('dragleave', () => row.classList.remove('is-dragover'));
      row.addEventListener('drop', e => {
        e.preventDefault();
        row.classList.remove('is-dragover');
        upload(e.dataTransfer.files[0]);
      });
      const buttons = el('div', 'image-drop-buttons');
      buttons.append(choose, find);
      row.append(preview, placeholder, buttons, el('small', 'image-drop-note', 'JPG, PNG or GIF (max 8 MB)'), crop, remove);
    } else {
      row.append(preview, placeholder, choose, find, crop, remove);
    }
    wrap.append(row);
    return {wrap, value: () => name};
  }

  return {uploadImage, uploadAndSave, imageSearchOn, openImageSearch, imagePicker};
}
