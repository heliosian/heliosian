function tierLabels() {
  const labels = location.hostname.split('.');
  return labels.length > 2 ? labels.slice(1) : labels;
}

export function currentApp() {
  const labels = location.hostname.split('.');
  return labels.length > 2 ? labels[0] : 'home';
}

export function appOrigin(key) {
  const tier = tierLabels();
  const host = key === 'home' ? tier : [key, ...tier];
  return location.protocol + '//' + host.join('.') + (location.port ? ':' + location.port : '');
}
