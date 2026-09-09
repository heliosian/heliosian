const res = await fetch('/auth/client');
const {clientId} = await res.json();

const onload = document.createElement('div');
onload.id = 'g_id_onload';
onload.setAttribute('data-client_id', clientId);
onload.setAttribute('data-login_uri', location.origin + '/auth/login');
onload.setAttribute('data-hosted_domain', 'heliosschool.org');
onload.setAttribute('data-auto_prompt', 'false');

const button = document.createElement('div');
button.className = 'g_id_signin';
button.setAttribute('data-type', 'standard');
button.setAttribute('data-theme', 'filled_black');
button.setAttribute('data-size', 'large');
button.setAttribute('data-shape', 'pill');
button.setAttribute('data-text', 'continue_with');
button.setAttribute('data-width', '300');

const holder = document.querySelector('#signin');
holder.before(onload);
holder.append(button);

const script = document.createElement('script');
script.src = 'https://accounts.google.com/gsi/client';
script.async = true;
document.head.append(script);
