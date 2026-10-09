'use strict';

const form = document.getElementById('loginForm');
const errorBox = document.getElementById('loginError');
const button = document.getElementById('loginBtn');

const messages = {
  401: 'Identifiants incorrects.',
  429: 'Trop de tentatives échouées, réessayez dans une minute.',
};

// Already logged in, or authentication disabled: go straight to the app.
fetch('/api/session').then((res) => { if (res.ok) location.replace('/'); });

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  errorBox.hidden = true;
  button.disabled = true;
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ user: form.user.value, password: form.password.value }),
    });
    if (res.ok) {
      location.replace('/');
      return;
    }
    const body = await res.json().catch(() => ({}));
    errorBox.textContent = messages[res.status] || body.error || 'Erreur ' + res.status;
  } catch (err) {
    errorBox.textContent = 'Serveur injoignable : ' + err.message;
  }
  errorBox.hidden = false;
  button.disabled = false;
  form.password.select();
});
