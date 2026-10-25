import {photoUrl, thumbOf, fullOf} from './state.js';
import {el, svg} from '/elements.js';
import {uploadPhoto, movePhoto, removePhoto, cropPhoto} from './edit.js';
import {openCropTool, openPhotoLightbox} from '/crop.js';

export const maxPhotos = 5;

export function cropped(photo) {
  return Boolean(photo && photo.crop_width);
}

export function togglePhotoMenu(menu) {
  menu.hidden = !menu.hidden;
  if (menu.hidden) {
    return;
  }
  menu.rebuild();
  menu.style.transform = '';
  const rect = menu.getBoundingClientRect();
  const margin = 8;
  let shift = 0;
  if (rect.left < margin) {
    shift = margin - rect.left;
  } else if (rect.right > window.innerWidth - margin) {
    shift = window.innerWidth - margin - rect.right;
  }
  if (shift) {
    menu.style.transform = `translateX(${shift}px)`;
  }
}

export function cropBadge() {
  const badge = el('div', 'photo-crop-badge');
  badge.title = 'Manually cropped';
  badge.append(svg('expand'), el('span', '', 'View full photo'));
  return badge;
}

export function photoMenu(photos, getPhoto, editing, status) {
  const menu = el('div', 'photo-menu');
  menu.hidden = true;
  menu.rebuild = () => {
    menu.replaceChildren();
    const photo = getPhoto();
    const at = photos.findIndex(ph => ph.id === photo.id);
    const item = (iconName, label, action) => {
      const btn = el('button', 'photo-menu-item');
      btn.type = 'button';
      btn.append(svg(iconName), el('span', '', label));
      btn.addEventListener('click', e => {
        e.stopPropagation();
        menu.hidden = true;
        action();
      });
      menu.append(btn);
    };
    item('eye', 'View photo', () => openPhotoLightbox(fullOf(photo)));
    if (at > 0) {
      item('star', 'Set as primary', () => movePhoto(photos, at, 0, status));
    }
    if (editing) {
      item('trash', 'Delete photo', () => {
        if (confirm('Remove this photo?')) {
          removePhoto(photo, status);
        }
      });
    }
    if (photo.reencode) {
      item('crop', 'Crop photo', () => openCropTool(fullOf(photo), true, box => cropPhoto(photo, box, status)));
    }
  };
  menu.rebuild();
  return menu;
}

export function familyPhotoMenu(photo, status) {
  const menu = el('div', 'photo-menu');
  menu.hidden = true;
  // togglePhotoMenu calls rebuild on every open; a family's items never change.
  menu.rebuild = () => {};
  const item = (iconName, label, action) => {
    const btn = el('button', 'photo-menu-item');
    btn.type = 'button';
    btn.append(svg(iconName), el('span', '', label));
    btn.addEventListener('click', e => {
      e.stopPropagation();
      menu.hidden = true;
      action();
    });
    menu.append(btn);
  };
  item('eye', 'View photo', () => openPhotoLightbox(fullOf(photo)));
  if (photo.reencode) {
    item('crop', 'Crop photo', () => openCropTool(fullOf(photo), false, box => cropPhoto(photo, box, status)));
  }
  return menu;
}

export function photoGrid(p, photos, editable, editing, heroImg, status, onPreview) {
  const grid = el('div', 'photo-grid');

  const previewPhoto = photo => {
    if (heroImg) {
      heroImg.src = photoUrl(photo);
    }
    if (onPreview) {
      onPreview(photo);
    }
  };

  const tileIds = () => [...grid.querySelectorAll('.photo-slot')].map(t => t.dataset.id).filter(Boolean);

  const restore = order => {
    const addTile = grid.querySelector('.photo-slot-add');
    for (const id of order) {
      grid.insertBefore(grid.querySelector(`.photo-slot[data-id="${CSS.escape(id)}"]`), addTile);
    }
  };

  function wireDrag(tile, photo) {
    let pointerId = null;
    let dragging = false;
    let startOrder = null;
    let startX = 0;
    let startY = 0;
    tile.addEventListener('pointerdown', e => {
      if (e.pointerType === 'mouse' && e.button !== 0) {
        return;
      }
      e.preventDefault();
      pointerId = e.pointerId;
      startX = e.clientX;
      startY = e.clientY;
    });
    tile.addEventListener('pointermove', e => {
      if (pointerId !== e.pointerId) {
        return;
      }
      if (!dragging) {
        if (Math.hypot(e.clientX - startX, e.clientY - startY) < 6) {
          return;
        }
        dragging = true;
        startOrder = tileIds();
        tile.setPointerCapture(pointerId);
        tile.classList.add('dragging');
      }
      const addTile = grid.querySelector('.photo-slot-add');
      for (const sib of grid.querySelectorAll('.photo-slot')) {
        if (sib === tile) {
          continue;
        }
        const rect = sib.getBoundingClientRect();
        const mid = rect.left + rect.width / 2;
        const tileIsBefore = Boolean(sib.compareDocumentPosition(tile) & Node.DOCUMENT_POSITION_PRECEDING);
        if (e.clientX < mid && !tileIsBefore) {
          grid.insertBefore(tile, sib);
          break;
        }
        if (e.clientX > mid && tileIsBefore) {
          grid.insertBefore(tile, sib.nextSibling === addTile ? addTile : sib.nextSibling);
          break;
        }
      }
    });
    const finish = async e => {
      if (pointerId !== e.pointerId) {
        return;
      }
      if (dragging) {
        tile.classList.remove('dragging');
        const before = startOrder;
        const to = tileIds().indexOf(photo.id);
        const from = before.indexOf(photo.id);
        if (from !== to && !await movePhoto(photos, from, to, status)) {
          restore(before);
        }
      } else {
        previewPhoto(photo);
      }
      pointerId = null;
      dragging = false;
      startOrder = null;
    };
    tile.addEventListener('pointerup', finish);
    tile.addEventListener('pointercancel', () => {
      if (dragging && startOrder) {
        restore(startOrder);
        tile.classList.remove('dragging');
      }
      pointerId = null;
      dragging = false;
      startOrder = null;
    });
  }

  function addTile() {
    const tile = el('label', 'photo-slot photo-slot-add');
    tile.title = 'Add photo';
    tile.append(el('span', '', '+'));
    const input = el('input');
    input.type = 'file';
    input.accept = 'image/*';
    input.hidden = true;
    input.addEventListener('change', () => {
      if (input.files.length) {
        uploadPhoto({person: p.id}, input.files[0], status);
      }
    });
    tile.append(input);
    return tile;
  }

  photos.forEach((photo, i) => {
    const tile = el('div', 'photo-slot' + (i === 0 ? ' photo-slot-primary' : ''));
    tile.dataset.id = photo.id;
    const face = el('img');
    face.src = thumbOf(photo);
    face.alt = '';
    face.draggable = false;
    tile.append(face);
    if (editing) {
      const del = el('button', 'photo-slot-delete', '×');
      del.type = 'button';
      del.title = 'Remove photo';
      del.addEventListener('pointerdown', e => e.stopPropagation());
      del.addEventListener('click', e => {
        e.stopPropagation();
        if (confirm('Remove this photo?')) {
          removePhoto(photo, status);
        }
      });
      tile.append(del);
    }
    if (editable) {
      tile.classList.add('photo-slot-draggable');
      wireDrag(tile, photo);
    } else {
      tile.addEventListener('click', () => previewPhoto(photo));
    }
    grid.append(tile);
  });
  if (editable && photos.length < maxPhotos) {
    grid.append(addTile());
  }

  const wrap = el('div');
  wrap.append(grid);
  return wrap;
}
