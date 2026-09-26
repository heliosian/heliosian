import {state} from './state.js';
import {el, svg, button, toast} from './dom.js';
import {imageTools} from '/images.js';

export const {uploadImage, imageSearchOn, openImageSearch} = imageTools('/api/calendar', {state, toast});

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
  const find = button('', 'search', 'icon-button admin-image-find', () => openImageSearch(t.name, picked => {
    t.image = picked.name;
    t.imageUrl = picked.url;
    onChange();
  }));
  find.setAttribute('aria-label', 'Find an image');
  find.title = 'Find an image';
  if (imageSearchOn()) {
    wrap.append(find);
  }
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
