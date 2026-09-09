export function initSidebar() {
  // Delegated on the sidebar, not bound per-tab, because the Super Admins and Spoof
  // Mode tabs don't exist in the DOM until ensureSuperAdminUI() adds them.
  document.querySelector('.sidebar').addEventListener('click', e => {
    const tab = e.target.closest('.tab');
    if (!tab) {
      return;
    }
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    tab.classList.add('active');
    document.querySelectorAll('.panel').forEach(p => p.hidden = true);
    document.querySelector('#panel-' + tab.dataset.panel).hidden = false;
    document.querySelector('#sidebar-toggle-label').textContent = tab.textContent;
    document.querySelector('.sidebar').classList.remove('menu-open');
  });

  // Mobile-only: the sidebar collapses to this toggle, which opens the same tab
  // list (see .sidebar-menu) as a dropdown instead of the full always-on list
  // desktop has room for.
  document.querySelector('#sidebar-toggle').addEventListener('click', e => {
    e.stopPropagation();
    document.querySelector('.sidebar').classList.toggle('menu-open');
  });
  document.addEventListener('click', e => {
    const sidebar = document.querySelector('.sidebar');
    if (sidebar.classList.contains('menu-open') && !sidebar.contains(e.target)) {
      sidebar.classList.remove('menu-open');
    }
  });
}
