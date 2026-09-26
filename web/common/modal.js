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

let current = null;
let reload = null;
const stack = [];

const overlay = () => document.querySelector('#modal-overlay');
const form = () => document.querySelector('#modal');

export function closeModal() {
  const below = stack.pop();
  if (below) {
    form().className = below.className;
    form().replaceChildren(...below.nodes);
    current = below.state;
    return;
  }
  overlay().hidden = true;
  form().replaceChildren();
  current = null;
}

function dismissable() {
  return !(current && current.required);
}

function setStatus(message, error) {
  const status = form().querySelector('.save-status');
  status.textContent = message;
  status.classList.toggle('error', Boolean(error));
}

export function initModal(load) {
  reload = load;
  overlay().addEventListener('click', e => {
    if (e.target === overlay() && dismissable()) {
      closeModal();
    }
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && dismissable()) {
      closeModal();
    }
  });
  form().addEventListener('submit', async e => {
    e.preventDefault();
    if (!current || !current.submit) {
      return;
    }
    // Taken now: a step that opens the next modal replaces it while its submit runs.
    const saving = current;
    setStatus(saving.working || 'Saving…');
    try {
      const saved = await saving.submit();
      if (saving.stay) {
        return;
      }
      closeModal();
      await reload();
      if (saving.afterSave) {
        saving.afterSave(saved);
      }
    } catch (err) {
      setStatus(err.message, true);
    }
  });
}

function actionButton(className, label, onClick) {
  const node = el('button', className, label);
  node.type = 'button';
  node.addEventListener('click', onClick);
  return node;
}

export function openModal(title, fields, options = {}) {
  const f = form();
  if (current && !options.replace) {
    stack.push({nodes: [...f.children], state: current, className: f.className});
  }
  f.replaceChildren();
  f.className = 'modal';
  f.classList.toggle('modal-wide', options.wide === true);
  f.classList.toggle('modal-table', options.wide === 'table');
  f.classList.toggle('modal-person', options.wide === 'person');
  const header = el('div', 'modal-header');
  header.append(el('h2', '', title));
  if (!options.required) {
    const close = actionButton('modal-close', '×', closeModal);
    close.setAttribute('aria-label', 'Close');
    header.append(close);
  }
  f.append(header, ...fields);
  const actions = el('div', 'modal-actions');
  if (options.actions === false) {
    actions.hidden = true;
  } else if (options.submit) {
    const save = el('button', 'button', options.saveLabel || 'Save');
    save.type = 'submit';
    actions.append(save);
    if (options.alternate) {
      actions.append(actionButton('button button-secondary', options.alternate.label, options.alternate.onClick));
    }
    if (!options.hideCancel) {
      actions.append(actionButton('button button-secondary', 'Cancel', closeModal));
    }
  } else {
    actions.append(actionButton('button', 'Done', closeModal));
  }
  if (options.onDelete) {
    actions.append(actionButton('danger-button', options.deleteLabel || 'Delete', async () => {
      if (!confirm(options.confirmDelete)) {
        return;
      }
      setStatus('Deleting…');
      try {
        await options.onDelete();
        closeModal();
        await reload();
        if (options.afterDelete) {
          options.afterDelete();
        }
      } catch (err) {
        setStatus(err.message, true);
      }
    }));
  }
  actions.append(el('span', 'save-status'));
  f.append(actions);
  current = {submit: options.submit, afterSave: options.afterSave, stay: options.stay, working: options.working, required: options.required};
  overlay().hidden = false;
  const first = f.querySelector('input:not([type=hidden]):not([type=file]):not([type=checkbox]), textarea, select');
  if (first) {
    first.focus();
  }
}

export function popup(title, node, {wide = false} = {}) {
  const layer = el('div', 'modal-overlay modal-sheet');
  const box = el('div', 'modal' + (wide ? ' modal-wide' : ''));
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
  return {box, shut};
}
