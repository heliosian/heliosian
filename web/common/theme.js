// The colouring an admin gave this app's page (internal/theme): the rail
// and the page, each one colour or a gradient running down from it to a
// second, the colour of the rail's words, and two pictures, the rail's
// logo and the art at its foot. applyTheme sets --sidebar-bg
// and --page-bg on the root - each
// always a linear-gradient, a solid being one from a colour to itself, so
// the value is an image and can sit in a background-image list beside an
// app's own picture - and marks the root is-themed-sidebar and
// is-themed-page while one is set. Every app's stylesheet reads them as
// `background: var(--sidebar-bg, <its own>)`, so a blank leaves its own
// look. Every app calls it once its model is in.
export function applyTheme(theme) {
  const root = document.documentElement;
  const {sidebar, sidebarEnd, sidebarText, page, pageEnd, logo, sidebarImage} = theme || {};
  const set = (name, flag, from, to) => {
    if (from) {
      root.style.setProperty(name, `linear-gradient(180deg, ${from}, ${to || from})`);
    } else {
      root.style.removeProperty(name);
    }
    root.classList.toggle(flag, Boolean(from));
  };
  set('--sidebar-bg', 'is-themed-sidebar', sidebar, sidebarEnd);
  set('--page-bg', 'is-themed-page', page, pageEnd);
  // The rail's words: a plain colour, --sidebar-fg.
  if (sidebarText) {
    root.style.setProperty('--sidebar-fg', sidebarText);
  } else {
    root.style.removeProperty('--sidebar-fg');
  }
  root.classList.toggle('is-themed-sidebar-text', Boolean(sidebarText));
  // The rail's logo: the first picture in the rail's brand slot takes the
  // uploaded one, and gets its own back when there is none.
  const brand = document.querySelector('.sidebar .brand img, .drawer .brand img, .brand-lockup');
  if (brand) {
    if (logo) {
      if (!brand.dataset.ownSrc) {
        brand.dataset.ownSrc = brand.getAttribute('src');
      }
      brand.src = '/' + logo;
    } else if (brand.dataset.ownSrc) {
      brand.src = brand.dataset.ownSrc;
    }
  }
  // The art at the rail's foot: --sidebar-art is a url() for a stylesheet
  // that paints it as a background layer, and a rail that holds it as an
  // element (Heliosian's .sidebar-art) swaps the picture the same way.
  if (sidebarImage) {
    root.style.setProperty('--sidebar-art', `url("/${sidebarImage}")`);
  } else {
    root.style.removeProperty('--sidebar-art');
  }
  root.classList.toggle('is-themed-sidebar-art', Boolean(sidebarImage));
  const art = document.querySelector('.sidebar-art');
  if (art) {
    if (sidebarImage) {
      if (!art.dataset.ownSrc) {
        art.dataset.ownSrc = art.getAttribute('src');
      }
      art.src = '/' + sidebarImage;
    } else if (art.dataset.ownSrc) {
      art.src = art.dataset.ownSrc;
    }
  }
}
