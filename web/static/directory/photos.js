import {el, svg, thumbUrl} from './dom.js';
import {submitMedia, submitPhotoOrder, submitCrop} from './edit.js';

// The hero photo is always a square crop (same treatment as every other avatar in
// the app), which can crop a photo awkwardly - this is the escape hatch: click it
// to see the whole, uncropped image in an overlay. Closes on click-anywhere or Esc.
export function openPhotoLightbox(url) {
  const overlay = el('div', 'photo-lightbox');
  const img = el('img');
  img.src = url;
  img.alt = '';
  overlay.append(img);
  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  };
  function onKey(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
  overlay.addEventListener('click', close);
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
}

// togglePhotoMenu opens/closes a photoMenu. Its CSS anchors with `right: 0`,
// which keeps it on screen for a trigger near the right edge (e.g. the "more"
// menu, always top-right) but a photo tile can sit anywhere across a grid - on a
// narrow/mobile viewport a tile nearer the left edge pushes the menu's left side
// past x=0. After opening, nudge it back on screen with a transform once we can
// measure where the pure-CSS position actually landed.
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

// cropBadge marks the hero photo as showing a manually cropped square rather
// than the plain auto-crop of the original - grid tiles are small enough that
// it would just add clutter, so this is hero-only. A photo's url differs from
// its originalUrl exactly when a crop is currently applied and resolves (see
// attachBlobs, internal/directory/load.go) - that's the signal used here rather
// than a separate field, since it's already exactly what "has a crop" means.
// Purely decorative (pointer-events: none in CSS) - the hero image underneath
// already opens the original in the lightbox on click, so the badge just
// labels that behavior rather than duplicating it.
export function cropBadge() {
  const badge = el('div', 'photo-crop-badge');
  badge.title = 'Manually cropped';
  badge.append(svg('expand'), el('span', '', 'View full photo'));
  return badge;
}

// photoMenu builds the Facebook-style "View photo / Set as primary / Delete
// photo / Crop photo" popup for whichever photo the hero is currently showing.
// Hidden by default; the caller wires a trigger to call togglePhotoMenu(menu),
// which rebuilds the item list fresh on every open (via menu.rebuild, set here)
// and appends the menu inside that trigger's own position:relative container.
// Reuses the app-wide outside-click/Escape closer (in chrome.js, alongside
// .more-menu/.card-menu) for free - no bespoke close handling here.
//
// getPhoto() returns the photo currently shown in the hero, which isn't always
// p.photos[0]: tapping a grid tile (photoGrid's previewPhoto) swaps the hero's
// preview without touching this menu, so the menu has to ask fresh each time it
// opens rather than close over one photo at build time - otherwise it would
// keep acting on the primary photo even while a different one is on screen.
// "Set as primary" is hidden when the current photo already is; a grid tile's
// own click never opens a menu at all (see photoGrid) - drag it to the front,
// or preview it and use this menu, both end up here. Delete only shows in
// editing mode, same convention as the grid tiles' own delete "x" -
// destructive actions wait for edit mode.
export function photoMenu(p, getPhoto, editing, status) {
  const menu = el('div', 'photo-menu');
  menu.hidden = true;
  menu.rebuild = () => {
    menu.replaceChildren();
    const photo = getPhoto();
    const photos = p.photos || [];
    const isPrimary = !photos.length || photo.name === photos[0].name;
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
    item('eye', 'View photo', () => openPhotoLightbox(photo.originalUrl));
    if (!isPrimary) {
      item('star', 'Set as primary', () => {
        const order = [photo.name, ...photos.map(ph => ph.name).filter(n => n !== photo.name)];
        submitPhotoOrder(p.email, order, status);
      });
    }
    if (editing) {
      item('trash', 'Delete photo', () => {
        // The school portrait is harder to get back than a self-uploaded photo, so
        // it gets a stronger warning, but either way this is permanent - confirm first.
        const message = photo.source === 'veracross'
          ? 'This is the school portrait from Veracross. Remove it anyway?'
          : 'Remove this photo?';
        if (!confirm(message)) {
          return;
        }
        status.textContent = 'Removing…';
        const order = photos.map(ph => ph.name).filter(name => name !== photo.name);
        submitPhotoOrder(p.email, order, status);
      });
    }
    item('crop', 'Crop photo', () => openCropTool(photo.originalUrl, true,
      blob => submitCrop('person', p.email, photo.name, blob, status)));
  };
  menu.rebuild();
  return menu;
}

// familyPhotoMenu is photoMenu's family equivalent: a family only ever has one
// photo (no primary to set, no gallery to delete from), so it's just "View
// photo" and "Crop photo". Only built when the caller may edit the family -
// same convention as photoMenu, whose caller (the person hero) does the same
// check before constructing one at all.
export function familyPhotoMenu(family, status) {
  const menu = el('div', 'photo-menu');
  menu.hidden = true;
  // togglePhotoMenu (shared with photoMenu above) always calls menu.rebuild()
  // on open - a family's items never change between opens, so there's
  // nothing to rebuild, but the hook still needs to exist.
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
  item('eye', 'View photo', () => openPhotoLightbox(family.originalPhotoUrl || family.photoUrl));
  // Unlike a profile photo, a family photo isn't shown anywhere as a fixed
  // shape (a circle, a square tile), so its crop tool is fully freeform (see
  // openCropTool's square param) rather than locked to a square.
  item('crop', 'Crop photo', () => openCropTool(family.originalPhotoUrl || family.photoUrl, false,
    blob => submitCrop('family', family.key, '', blob, status)));
  return menu;
}

// openCropTool is a full-screen crop editor for one photo. When square is
// true the frame is locked to 1:1, same as a person's profile photo has
// always been; when false the frame is fully freeform (any position, any
// size, any shape), for a family photo, which isn't shown anywhere as a fixed
// shape the way a profile photo is. There's no stored crop rectangle to
// restore - only the resulting cropped image is saved - so cropping a photo
// that already has a crop just starts fresh from the original and replaces
// it. onSave(blob) performs the actual upload and resolves to whether it
// succeeded; the tool only closes itself on success.
function openCropTool(imageUrl, square, onSave) {
  const overlay = el('div', 'crop-overlay');
  const panel = el('div', 'crop-panel');
  const stage = el('div', 'crop-stage');
  const img = el('img', 'crop-image');
  img.src = imageUrl;
  img.alt = '';
  const frame = el('div', 'crop-frame');
  const handleEls = ['nw', 'ne', 'sw', 'se'].map(corner => {
    const handle = el('div', 'crop-handle crop-handle-' + corner);
    handle.dataset.corner = corner;
    return handle;
  });
  frame.append(...handleEls);
  const maskTop = el('div', 'crop-mask');
  const maskBottom = el('div', 'crop-mask');
  const maskLeft = el('div', 'crop-mask');
  const maskRight = el('div', 'crop-mask');
  stage.append(img, maskTop, maskLeft, maskRight, maskBottom, frame);
  const actions = el('div', 'crop-actions');
  const cancel = el('button', 'media-button', 'Cancel');
  cancel.type = 'button';
  const save = el('button', 'media-button primary', 'Save crop');
  save.type = 'button';
  actions.append(cancel, save);
  panel.append(stage, actions);
  overlay.append(panel);

  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  };
  function onKey(e) {
    if (e.key === 'Escape') {
      close();
    }
  }
  overlay.addEventListener('click', e => {
    // Only the dark backdrop closes on click - not the panel, stage, or frame,
    // which all live inside it and need their own clicks/drags to work.
    if (e.target === overlay) {
      close();
    }
  });
  cancel.addEventListener('click', close);
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);

  const minSide = 60;
  let left = 0;
  let top = 0;
  let width = 0;
  let height = 0;

  function render() {
    frame.style.left = left + 'px';
    frame.style.top = top + 'px';
    frame.style.width = width + 'px';
    frame.style.height = height + 'px';
    maskTop.style.cssText = `top:0; left:0; right:0; height:${top}px`;
    maskBottom.style.cssText = `top:${top + height}px; left:0; right:0; bottom:0`;
    maskLeft.style.cssText = `top:${top}px; left:0; width:${left}px; height:${height}px`;
    maskRight.style.cssText = `top:${top}px; left:${left + width}px; right:0; height:${height}px`;
  }

  // In square mode every call passes nextWidth === nextHeight (see the drag
  // handlers below, which move both in lockstep) - but clamping each against
  // its own axis independently would still let the frame outgrow whichever
  // axis is shorter (the stage is rarely itself square) and stop being
  // square, so both are first capped to the same shared bound before the
  // per-axis clamp below, mirroring the single min(side, width, height) the
  // old single-side version used.
  function setFrame(nextLeft, nextTop, nextWidth, nextHeight) {
    const stageRect = stage.getBoundingClientRect();
    if (square) {
      const maxSide = Math.min(stageRect.width, stageRect.height);
      nextWidth = Math.min(nextWidth, maxSide);
      nextHeight = Math.min(nextHeight, maxSide);
    }
    width = Math.max(minSide, Math.min(nextWidth, stageRect.width));
    height = Math.max(minSide, Math.min(nextHeight, stageRect.height));
    left = Math.max(0, Math.min(nextLeft, stageRect.width - width));
    top = Math.max(0, Math.min(nextTop, stageRect.height - height));
    render();
  }

  function init() {
    const stageRect = stage.getBoundingClientRect();
    if (square) {
      const initialSide = Math.min(stageRect.width, stageRect.height);
      setFrame((stageRect.width - initialSide) / 2, (stageRect.height - initialSide) / 2, initialSide, initialSide);
    } else {
      const initialWidth = stageRect.width * 0.9;
      const initialHeight = stageRect.height * 0.9;
      setFrame((stageRect.width - initialWidth) / 2, (stageRect.height - initialHeight) / 2, initialWidth, initialHeight);
    }
  }
  if (img.complete && img.naturalWidth) {
    init();
  } else {
    img.addEventListener('load', init);
  }

  // Shared pointer-drag wiring for both moving the frame and resizing it from a
  // corner handle - same pointerdown/pointermove/pointerup(+capture) pattern
  // photoGrid's own drag-to-reorder already uses, for mobile/touch reliability.
  function drag(target, onMove) {
    target.addEventListener('pointerdown', e => {
      e.preventDefault();
      e.stopPropagation();
      const pointerId = e.pointerId;
      const startX = e.clientX;
      const startY = e.clientY;
      const startLeft = left;
      const startTop = top;
      const startWidth = width;
      const startHeight = height;
      target.setPointerCapture(pointerId);
      const move = m => {
        if (m.pointerId !== pointerId) {
          return;
        }
        onMove(m.clientX - startX, m.clientY - startY, startLeft, startTop, startWidth, startHeight);
      };
      const up = u => {
        if (u.pointerId !== pointerId) {
          return;
        }
        target.removeEventListener('pointermove', move);
        target.removeEventListener('pointerup', up);
        target.removeEventListener('pointercancel', up);
      };
      target.addEventListener('pointermove', move);
      target.addEventListener('pointerup', up);
      target.addEventListener('pointercancel', up);
    });
  }

  drag(frame, (dx, dy, startLeft, startTop, startWidth, startHeight) => {
    setFrame(startLeft + dx, startTop + dy, startWidth, startHeight);
  });
  for (const handle of handleEls) {
    const corner = handle.dataset.corner;
    drag(handle, (dx, dy, startLeft, startTop, startWidth, startHeight) => {
      if (square) {
        // A square frame must grow/shrink the same amount on both axes to stay
        // square, so both corners being dragged move by one shared delta - the
        // larger of the two axis deltas, so the frame always follows whichever
        // direction the pointer moved furthest in.
        let delta;
        let nextLeft = startLeft;
        let nextTop = startTop;
        if (corner === 'se') {
          delta = Math.max(dx, dy);
        } else if (corner === 'nw') {
          delta = Math.max(-dx, -dy);
          nextLeft = startLeft - delta;
          nextTop = startTop - delta;
        } else if (corner === 'ne') {
          delta = Math.max(dx, -dy);
          nextTop = startTop - delta;
        } else {
          delta = Math.max(-dx, dy);
          nextLeft = startLeft - delta;
        }
        setFrame(nextLeft, nextTop, startWidth + delta, startHeight + delta);
        return;
      }
      // Freeform: each corner drags its own two edges independently, with no
      // coupling between width and height.
      let nextLeft = startLeft;
      let nextTop = startTop;
      let nextWidth = startWidth;
      let nextHeight = startHeight;
      if (corner === 'se') {
        nextWidth = startWidth + dx;
        nextHeight = startHeight + dy;
      } else if (corner === 'nw') {
        nextLeft = startLeft + dx;
        nextTop = startTop + dy;
        nextWidth = startWidth - dx;
        nextHeight = startHeight - dy;
      } else if (corner === 'ne') {
        nextTop = startTop + dy;
        nextWidth = startWidth + dx;
        nextHeight = startHeight - dy;
      } else {
        nextLeft = startLeft + dx;
        nextWidth = startWidth - dx;
        nextHeight = startHeight + dy;
      }
      setFrame(nextLeft, nextTop, nextWidth, nextHeight);
    });
  }

  save.addEventListener('click', () => {
    const stageRect = stage.getBoundingClientRect();
    const scaleX = img.naturalWidth / stageRect.width;
    const scaleY = img.naturalHeight / stageRect.height;
    const sx = left * scaleX;
    const sy = top * scaleY;
    const sWidth = width * scaleX;
    const sHeight = height * scaleY;
    const maxOut = 1600;
    const shrink = Math.max(sWidth, sHeight) > maxOut ? maxOut / Math.max(sWidth, sHeight) : 1;
    const outWidth = Math.max(1, Math.round(sWidth * shrink));
    const outHeight = Math.max(1, Math.round(sHeight * shrink));
    const canvas = document.createElement('canvas');
    canvas.width = outWidth;
    canvas.height = outHeight;
    canvas.getContext('2d').drawImage(img, sx, sy, sWidth, sHeight, 0, 0, outWidth, outHeight);
    save.disabled = true;
    save.textContent = 'Saving…';
    canvas.toBlob(async blob => {
      if (!blob) {
        save.disabled = false;
        save.textContent = 'Save crop';
        return;
      }
      const ok = await onSave(blob);
      if (ok) {
        close();
      } else {
        save.disabled = false;
        save.textContent = 'Save crop';
      }
    }, 'image/jpeg', 0.92);
  });
}

// photoGrid shows up to 5 photo tiles (already primary-first - the server always
// returns them in the order that's shown everywhere). Anyone who may edit the
// record can always drag a tile to reorder it, or add one while under the cap -
// no separate edit mode needed for either. The delete "x" on each tile is the one
// control that waits for edit mode, same convention as every other field on this
// page, since it's destructive. Clicking (rather than dragging) a tile - for
// anyone, editable or not - just shows that photo bigger in the hero image above
// (onPreview, if given, also redirects the hero's own View/Set as primary/Delete/
// Crop menu to that photo - see photoMenu's getPhoto param); it changes nothing
// until a drag actually reorders something, or an action is chosen from that
// menu. Tiles don't get a menu of their own - no room to open one well on a
// narrow screen - so cropping or deleting a non-primary photo means previewing
// or dragging it to the front first, either of which puts it in reach of the
// hero's menu. Reordering is pointer-events based rather than native HTML5
// drag-and-drop, since the latter doesn't work reliably on mobile/touch (iOS
// Safari in particular) and this app is used heavily on phones. status is shared
// with the hero photo's own menu so both report errors in the same place.
export function photoGrid(p, editable, editing, heroImg, status, onPreview) {
  const grid = el('div', 'photo-grid');

  const previewPhoto = photo => {
    if (heroImg) {
      heroImg.src = photo.url;
    }
    if (onPreview) {
      onPreview(photo);
    }
  };

  const currentOrder = () => [...grid.querySelectorAll('.photo-slot')].map(t => t.dataset.name);

  const commitOrder = (revertOrder) => {
    submitPhotoOrder(p.email, currentOrder(), status, () => {
      const addTile = grid.querySelector('.photo-slot-add');
      for (const name of revertOrder) {
        grid.insertBefore(grid.querySelector(`.photo-slot[data-name="${CSS.escape(name)}"]`), addTile);
      }
    });
  };

  const deletePhoto = (photo) => {
    status.textContent = 'Removing…';
    submitPhotoOrder(p.email, currentOrder().filter(name => name !== photo.name), status);
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
      // Without this, holding and moving over the photo also kicks off the browser's
      // own image-drag ghost and text/image selection highlight, fighting visually
      // with the custom drag below.
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
        startOrder = currentOrder();
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
    const finish = e => {
      if (pointerId !== e.pointerId) {
        return;
      }
      if (dragging) {
        tile.classList.remove('dragging');
        const newOrder = currentOrder();
        if (startOrder && newOrder.join(',') !== startOrder.join(',')) {
          commitOrder(startOrder);
        }
      } else {
        // A tap that never crossed the drag threshold: just preview it bigger.
        previewPhoto(photo);
      }
      pointerId = null;
      dragging = false;
      startOrder = null;
    };
    tile.addEventListener('pointerup', finish);
    tile.addEventListener('pointercancel', () => {
      if (dragging && startOrder) {
        const addTile = grid.querySelector('.photo-slot-add');
        for (const name of startOrder) {
          grid.insertBefore(grid.querySelector(`.photo-slot[data-name="${CSS.escape(name)}"]`), addTile);
        }
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
        submitMedia('person', p.email, 'photo', input.files[0], input.files[0].name, status);
      }
    });
    tile.append(input);
    return tile;
  }

  p.photos.forEach((photo, i) => {
    const tile = el('div', 'photo-slot' + (i === 0 ? ' photo-slot-primary' : ''));
    tile.dataset.name = photo.name;
    tile.title = photo.source === 'veracross' ? 'School portrait' : 'Uploaded photo';
    const face = el('img');
    face.src = thumbUrl(photo.url);
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
        // The school portrait is harder to get back than a self-uploaded photo, so
        // it gets a stronger warning, but either way this is permanent - confirm first.
        const message = photo.source === 'veracross'
          ? 'This is the school portrait from Veracross. Remove it anyway?'
          : 'Remove this photo?';
        if (!confirm(message)) {
          return;
        }
        deletePhoto(photo);
      });
      tile.append(del);
    }
    if (editable) {
      tile.classList.add('photo-slot-draggable');
      wireDrag(tile, photo);
    } else {
      // No drag wired up here to distinguish a tap from a drag, so a plain click is
      // always just a preview.
      tile.addEventListener('click', () => previewPhoto(photo));
    }
    grid.append(tile);
  });
  if (editable && p.photos.length < 5) {
    grid.append(addTile());
  }

  const wrap = el('div');
  wrap.append(grid);
  return wrap;
}
