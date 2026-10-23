import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthProvider';
import { cataloguesFor, I18nProvider, isLoaded, loadCatalogue, type LabelMode, type Locale } from './I18nProvider';

interface PrefsCtx {
  locale: Locale;
  labelMode: LabelMode;
  setLocale: (l: Locale) => Promise<void>;
  setLabelMode: (m: LabelMode) => Promise<void>;
}

const Ctx = createContext<PrefsCtx | null>(null);
const KEY = 'nd.locale';

export function initialLocale(): Locale {
  try {
    const v = localStorage.getItem(KEY);
    if (v === 'en' || v === 'si') return v;
  } catch {
    // storage blocked
  }
  return typeof navigator !== 'undefined' && navigator.language?.toLowerCase().startsWith('si') ? 'si' : 'en';
}

function remember(l: Locale) {
  try {
    localStorage.setItem(KEY, l);
  } catch {
    // storage blocked
  }
}

/** UI language and label mode: from the profile once signed in, else the device. */
export function PrefsProvider({ children }: { children: ReactNode }) {
  const { me, refreshMe } = useAuth();
  const [wanted, setWanted] = useState<{ locale: Locale; labelMode: LabelMode }>(() => ({ locale: initialLocale(), labelMode: 'single' }));
  // What is rendered: switches only once the needed catalogues are loaded.
  const [shown, setShown] = useState(wanted);
  const { locale, labelMode } = wanted;

  useEffect(() => {
    let live = true;
    const need = cataloguesFor(wanted.locale, wanted.labelMode);
    if (need.every(isLoaded)) {
      setShown(wanted);
      return;
    }
    Promise.all(need.map(loadCatalogue))
      .then(() => live && setShown(wanted))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [wanted]);
  const setLocaleState = (l: Locale) => setWanted((w) => ({ ...w, locale: l }));
  const setLabelModeState = (m: LabelMode) => setWanted((w) => ({ ...w, labelMode: m }));

  useEffect(() => {
    if (!me) return;
    const l = me.preferred_locale === 'si' ? 'si' : 'en';
    remember(l);
    setWanted({ locale: l, labelMode: me.label_mode === 'dual' ? 'dual' : 'single' });
  }, [me]);

  useEffect(() => {
    document.documentElement.lang = shown.locale === 'si' ? 'si' : 'en';
  }, [shown.locale]);

  const setLocale = useCallback(
    async (l: Locale) => {
      setLocaleState(l);
      remember(l);
      if (me) {
        await api.request('PATCH', '/api/v1/me', { body: { preferred_locale: l } });
        await refreshMe();
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [me, refreshMe],
  );

  const setLabelMode = useCallback(
    async (m: LabelMode) => {
      setLabelModeState(m);
      if (me) {
        await api.request('PATCH', '/api/v1/me', { body: { label_mode: m } });
        await refreshMe();
      }
    },
    [me, refreshMe],
  );

  const value = useMemo(() => ({ locale, labelMode, setLocale, setLabelMode }), [locale, labelMode, setLocale, setLabelMode]);
  return (
    <Ctx.Provider value={value}>
      <I18nProvider locale={shown.locale} labelMode={shown.labelMode}>
        {children}
      </I18nProvider>
    </Ctx.Provider>
  );
}

export function usePrefs(): PrefsCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error('usePrefs outside PrefsProvider');
  return v;
}
