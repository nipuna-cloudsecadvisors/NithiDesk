import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { AuthProvider } from './auth/AuthProvider';
import { loadCatalogue } from './i18n/I18nProvider';
import { initialLocale, PrefsProvider } from './i18n/Prefs';
import { QueueProvider } from './offline/QueueProvider';
import { ToastProvider } from './ui/components';
import './styles/app.css';

const root = document.getElementById('root');
const first = initialLocale();
// Load only the device language before the first paint (English if that fails).
const ready = loadCatalogue(first).catch(() => loadCatalogue('en'));
if (root) {
  void ready.then(() => createRoot(root).render(
    <StrictMode>
      <AuthProvider>
        <PrefsProvider>
          <QueueProvider>
            <ToastProvider>
              <App />
            </ToastProvider>
          </QueueProvider>
        </PrefsProvider>
      </AuthProvider>
    </StrictMode>,
  ));
}
