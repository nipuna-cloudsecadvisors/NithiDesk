import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import type { SearchResult } from '../api/types';
import { isClerk, useMe } from '../auth/AuthProvider';
import { useT } from '../i18n/I18nProvider';
import { usePrefs } from '../i18n/Prefs';
import { useQueue } from '../offline/QueueProvider';

export function LangToggle() {
  const { locale, setLocale } = usePrefs();
  const { t } = useT();
  return (
    <div className="lang-toggle" role="group" aria-label={t('settings.language')}>
      <button type="button" lang="en" aria-pressed={locale === 'en'} onClick={() => void setLocale('en').catch(() => undefined)}>
        {t('lang.en.short')}
      </button>
      <button type="button" lang="si" aria-pressed={locale === 'si'} onClick={() => void setLocale('si').catch(() => undefined)}>
        {t('lang.si.short')}
      </button>
    </div>
  );
}

/** Header search with grouped results and the "/" shortcut on desktop. */
export function GlobalSearch({ autoFocus, onDone }: { autoFocus?: boolean; onDone?: () => void }) {
  const { t, name } = useT();
  const [q, setQ] = useState('');
  const [res, setRes] = useState<SearchResult | null>(null);
  const [open, setOpen] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const box = useRef<HTMLDivElement>(null);
  const location = useLocation();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const typing = el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable);
      if (e.key === '/' && !typing && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        input.current?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    setOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    const term = q.trim();
    if (term.length < 2) {
      setRes(null);
      return;
    }
    const ctl = new AbortController();
    const timer = window.setTimeout(() => {
      api
        .get<SearchResult>('/api/v1/search', { query: { q: term }, signal: ctl.signal })
        .then((r) => {
          setRes(r);
          setOpen(true);
        })
        .catch(() => undefined);
    }, 220);
    return () => {
      ctl.abort();
      window.clearTimeout(timer);
    };
  }, [q]);

  const matters = res?.matters ?? [];
  const procs = res?.proceedings ?? [];
  const nothing = res && matters.length === 0 && procs.length === 0;

  return (
    <div
      className="search"
      ref={box}
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          setOpen(false);
          input.current?.blur();
        }
      }}
      onBlur={(e) => {
        if (!box.current?.contains(e.relatedTarget as Node | null)) setOpen(false);
      }}
    >
      <label className="visually-hidden" htmlFor="global-search">
        {t('search.label')}
      </label>
      <input
        ref={input}
        id="global-search"
        type="search"
        inputMode="search"
        autoComplete="off"
        autoFocus={autoFocus}
        placeholder={t('search.placeholder')}
        value={q}
        onChange={(e) => setQ(e.target.value)}
        onFocus={() => res && setOpen(true)}
      />
      <kbd aria-hidden="true">{t('search.shortcut')}</kbd>
      {open && res ? (
        <div className="search__results">
          {nothing ? <p className="search-hit muted">{t('search.none')}</p> : null}
          {procs.length > 0 ? (
            <div className="search-group">
              <h3>{t('search.proceedings')}</h3>
              {procs.map((h) => (
                <Link key={h.id} className="search-hit" to={`/matters/${h.matter_id}#p-${h.id}`} onClick={onDone}>
                  <strong>{h.case_number ?? h.matter_number}</strong>
                  {name(h.title)}
                  <span className="muted small"> {t(`search.matched.${h.matched_on}`)}</span>
                </Link>
              ))}
            </div>
          ) : null}
          {matters.length > 0 ? (
            <div className="search-group">
              <h3>{t('search.matters')}</h3>
              {matters.map((h) => (
                <Link key={h.id} className="search-hit" to={`/matters/${h.matter_id}`} onClick={onDone}>
                  <strong>{h.matter_number}</strong>
                  {name(h.title)}
                </Link>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function SideLink({ to, label, count }: { to: string; label: string; count?: number }) {
  return (
    <NavLink to={to}>
      <span>{label}</span>
      {count ? <span aria-hidden="true">{count}</span> : null}
    </NavLink>
  );
}

export function Shell({ children, withActionBar }: { children: ReactNode; withActionBar?: boolean }) {
  const me = useMe();
  const { t, tl, name } = useT();
  const { outstanding, online } = useQueue();
  const can = (c: string) => me.capabilities.includes(c as never);
  const clerk = isClerk(me);
  const navigate = useNavigate();

  return (
    <div className="shell">
      <a className="skip-link" href="#main">
        {t('common.skipToContent')}
      </a>
      <aside className="sidebar" aria-label={t('nav.label')}>
        <div className="sidebar__brand">
          {t('app.name')}
          <span className="sidebar__firm">{name(me.firm_name)}</span>
        </div>
        <nav>
          <SideLink to="/today" label={tl('nav.today')} />
          <SideLink to="/week" label={tl('nav.diary')} />
          {can('hearing.verify') || clerk ? <SideLink to="/verification" label={tl('nav.verification')} /> : null}
          {can('diary.court_list_check') ? <SideLink to="/court-list" label={tl('nav.courtList')} /> : null}
          <SideLink to="/outcomes-pending" label={tl('nav.outcomesPending')} />
          <SideLink to="/matters" label={tl('nav.matters')} />
          <SideLink to="/brief" label={tl('nav.brief')} />
          {outstanding > 0 ? <SideLink to="/outbox" label={tl('nav.outbox')} count={outstanding} /> : null}
          {can('import.run') ? <SideLink to="/imports" label={tl('nav.imports')} /> : null}
          {can('user.manage') ? (
            <>
              <hr className="sidebar__sep" />
              <SideLink to="/admin" label={tl('nav.admin')} />
            </>
          ) : null}
          <hr className="sidebar__sep" />
          <SideLink to="/settings" label={tl('nav.settings')} />
        </nav>
        <div className="sidebar__foot">{name(me.display_name)}</div>
      </aside>
      <div>
        <header className="topbar">
          <Link className="topbar__brand" to="/">
            {t('app.name')}
          </Link>
          <GlobalSearch />
          <LangToggle />
          <button type="button" className="btn btn--quiet btn--small no-print topbar__me" onClick={() => navigate('/settings')}>
            <span className="visually-hidden">{t('nav.settings')}</span>
            <span aria-hidden="true">{initials(name(me.display_name))}</span>
          </button>
        </header>
        {!online ? (
          <div className="offline-banner" role="status">
            {t('offline.banner')}
          </div>
        ) : null}
        <main id="main" className={withActionBar ? 'main main--with-actionbar' : 'main'} tabIndex={-1}>
          {children}
        </main>
        <nav className="bottomnav" aria-label={t('nav.label')}>
          <NavLink to={clerk ? '/verification' : '/today'}>
            <span className="icon" aria-hidden="true">
              ▤
            </span>
            {clerk ? t('nav.verificationShort') : t('nav.today')}
          </NavLink>
          <NavLink to="/search">
            <span className="icon" aria-hidden="true">
              ⌕
            </span>
            {t('nav.search')}
          </NavLink>
          <NavLink to="/queue">
            <span className="icon" aria-hidden="true">
              ☰
            </span>
            {outstanding > 0 ? t('nav.queueWithCount', { count: outstanding }) : t('nav.queue')}
          </NavLink>
          <NavLink to="/more">
            <span className="icon" aria-hidden="true">
              ⋯
            </span>
            {t('nav.more')}
          </NavLink>
        </nav>
      </div>
    </div>
  );
}

function initials(s: string): string {
  const parts = s.split(/\s+/).filter(Boolean);
  return parts
    .slice(-2)
    .map((p) => Array.from(p)[0] ?? '')
    .join('')
    .toUpperCase();
}
