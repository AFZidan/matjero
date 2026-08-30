import React from 'react';
import { createRoot } from 'react-dom/client';
import { directionFor, messages, type Locale } from './i18n/locales';
import './styles.css';

const locale = (new URLSearchParams(window.location.search).get('locale') === 'ar' ? 'ar' : 'en') satisfies Locale;
const copy = messages[locale];

document.documentElement.lang = locale;
document.documentElement.dir = directionFor(locale);

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <main className="shell">
      <h1>{copy.appName}</h1>
      <p>{copy.status}</p>
    </main>
  </React.StrictMode>
);
