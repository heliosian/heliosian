(function () {
  var q = location.search.match(/[?&]mode=(light|dark|system|quan)/);
  if (q) {
    var h = location.hostname.split('.');
    document.cookie = 'heliosian-mode=' + q[1] + '; path=/; max-age=31536000; SameSite=Lax' + (h.length > 1 ? '; domain=.' + h.slice(-2).join('.') : '');
  }
  var m = document.cookie.match(/(?:^|; )heliosian-mode=(light|dark|system|quan)/);
  var d = m && m[1] === 'dark' || (!(m && (m[1] === 'light' || m[1] === 'quan')) && matchMedia('(prefers-color-scheme: dark)').matches);
  if (m && m[1] === 'quan') {
    document.documentElement.dataset.theme = 'quan';
  } else if (d) {
    document.documentElement.dataset.theme = 'dark';
  }
})();
