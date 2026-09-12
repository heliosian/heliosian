// Cropping and the full-size view for a picture, ported from Helios Who?'s
// photos.js so the apps handle one the same way; HCA-Team and Heliosian share
// this copy (served from web/common/). The crop tool is freeform: a frame
// dragged over the image, corners to resize, and Save hands the caller a JPEG
// blob of what is inside it (at most 1600px a side). The styles it needs
// (.crop-* and .photo-lightbox) live in each app's own stylesheet.

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

// openPhotoLightbox shows the whole image over the page - the hero and card
// crops can cut a picture awkwardly, and this is the way to see all of it.
// Closes on click-anywhere or Esc.
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

export function openCropTool(imageUrl, square, onSave) {
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
  const cancel = el('button', 'button button-secondary', 'Cancel');
  cancel.type = 'button';
  const save = el('button', 'button', 'Save crop');
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
