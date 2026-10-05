import {openCropTool, openPhotoLightbox} from '/crop.js';
import {el, svg, toast, imageThumb, copyText} from '/elements.js';

function heroButton(icon, label, onClick) {
  const node = el('button', 'hero-action');
  node.type = 'button';
  node.title = label;
  node.setAttribute('aria-label', label);
  node.append(svg(icon));
  node.addEventListener('click', onClick);
  return node;
}

export function detailHero({imageUrl, title, path, className, stamp, extras = [], edit}) {
  const wrap = el('div', 'detail-hero');
  const picture = imageThumb(imageUrl, title, 'detail-hero-image ' + (className || ''));
  const actions = el('div', 'hero-actions');
  actions.append(heroButton('share', 'Share this page', async () => {
    const url = location.origin + path;
    if (navigator.share) {
      try {
        await navigator.share({title, url});
        return;
      } catch {
      }
    }
    copyText(url, 'Link copied');
  }));
  if (imageUrl) {
    picture.classList.add('is-openable');
    picture.addEventListener('click', () => openPhotoLightbox(imageUrl));
  }
  wrap.append(picture, actions);
  if (stamp) {
    wrap.append(stamp);
  }
  wrap.append(...extras);
  if (edit) {
    wrap.append(heroImageBar({image: edit.image, imageUrl, query: title, tools: edit.tools, save: edit.save}));
  }
  return wrap;
}

export function heroImageBar({image, imageUrl, query, tools, save}) {
  const bar = el('div', 'hero-image-bar');
  const put = async picked => {
    bar.replaceChildren(el('span', 'hero-image-status', 'Uploading…'));
    try {
      const made = await tools.uploadImage(picked);
      await save(made.name);
    } catch (err) {
      toast(err.message);
    }
  };
  const file = el('input');
  file.type = 'file';
  file.accept = 'image/*';
  file.hidden = true;
  file.addEventListener('change', () => {
    if (file.files.length) {
      put(file.files[0]);
    }
  });
  const holder = el('div', 'hero-image-menu-holder');
  const toggle = el('button', 'hero-image-action');
  toggle.type = 'button';
  toggle.setAttribute('aria-haspopup', 'menu');
  toggle.append(svg('image'), el('span', '', 'Edit image'), svg('chevron-down'));
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
  item('up', 'Upload image', () => file.click());
  if (tools.imageSearchOn()) {
    item('search', 'Find an image', () => tools.openImageSearch(query, picked => save(picked.name)));
  }
  if (image) {
    item('crop', 'Crop', () => openCropTool(imageUrl, false, async blob => {
      await put(new File([blob], 'crop.jpg', {type: 'image/jpeg'}));
      return true;
    }));
    item('trash', 'Remove', () => save(''));
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
  return bar;
}
