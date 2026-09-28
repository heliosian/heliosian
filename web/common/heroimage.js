import {openCropTool} from '/crop.js';
import {el, svg, toast} from '/elements.js';

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
