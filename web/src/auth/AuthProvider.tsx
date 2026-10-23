import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, ApiError } from '../api/client';
import type { AuthResponse, Capability, Me } from '../api/types';
import { clearReferenceCache } from '../api/reference';

export type AuthState =
  | { kind: 'booting' }
  | { kind: 'anon'; reason?: string }
  | { kind: 'mfa'; mfa: 'enroll_required' | 'verify_required' }
  | { kind: 'locked' }
  | { kind: 'ready'; me: Me };

interface AuthCtx {
  state: AuthState;
  me: Me | null;
  can: (c: Capability) => boolean;
  login: (email: string, password: string, firm?: string) => Promise<void>;
  afterMfa: (csrf: string) => Promise<void>;
  unlock: (pin: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshMe: () => Promise<void>;
}

const Ctx = createContext<AuthCtx | null>(null);

/** Phones and tablets get the mobile session (shorter idle, PIN unlock). */
export function detectClientKind(): 'mobile' | 'desktop' {
  if (typeof window === 'undefined') return 'desktop';
  const coarse = window.matchMedia?.('(pointer: coarse)').matches ?? false;
  return coarse && window.innerWidth < 1024 ? 'mobile' : 'desktop';
}

/** A coarse, non-identifying device label shown in the sessions list. */
export function deviceLabel(): string {
  const ua = navigator.userAgent;
  const os = /Android/i.test(ua)
    ? 'Android'
    : /iPhone|iPad|iPod/i.test(ua)
      ? 'iOS'
      : /Windows/i.test(ua)
        ? 'Windows'
        : /Mac OS X/i.test(ua)
          ? 'macOS'
          : /Linux/i.test(ua)
            ? 'Linux'
            : 'Device';
  const browser = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'Browser';
  return `${browser} on ${os}`;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ kind: 'booting' });

  const toMfaFromServer = useCallback(async () => {
    const s = await api.get<AuthResponse>('/api/v1/auth/state', { noAuthFlow: true });
    api.setCsrf(s.csrf_token);
    if (s.mfa_state === 'satisfied') return false;
    setState({ kind: 'mfa', mfa: s.mfa_state });
    return true;
  }, []);

  const loadMe = useCallback(async () => {
    try {
      const me = await api.get<Me>('/api/v1/me', { noAuthFlow: true });
      api.setCsrf(me.csrf_token);
      setState({ kind: 'ready', me });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        if (err.code === 'auth.mfa_required') {
          if (await toMfaFromServer().catch(() => false)) return;
        } else if (err.code === 'auth.session_locked') {
          setState({ kind: 'locked' });
          return;
        }
        setState({ kind: 'anon', reason: err.code });
        return;
      }
      throw err;
    }
  }, [toMfaFromServer]);

  useEffect(() => {
    loadMe().catch(() => setState({ kind: 'anon', reason: 'network' }));
  }, [loadMe]);

  useEffect(
    () =>
      api.onAuth((e) => {
        if (e.kind === 'login') {
          api.setCsrf('');
          setState({ kind: 'anon', reason: e.code });
        } else if (e.kind === 'locked') {
          setState({ kind: 'locked' });
        } else if (e.kind === 'mfa') {
          toMfaFromServer().catch(() => setState({ kind: 'anon' }));
        }
      }),
    [toMfaFromServer],
  );

  const login = useCallback(async (email: string, password: string, firm?: string) => {
    const body: Record<string, string> = {
      email,
      password,
      client_kind: detectClientKind(),
      device_label: deviceLabel(),
    };
    if (firm && firm.trim()) body.firm = firm.trim();
    const r = await api.post<AuthResponse>('/api/v1/auth/login', body, { noAuthFlow: true });
    api.setCsrf(r.csrf_token);
    clearReferenceCache();
    if (r.mfa_state === 'satisfied') await loadMe();
    else setState({ kind: 'mfa', mfa: r.mfa_state });
  }, [loadMe]);

  const afterMfa = useCallback(
    async (csrf: string) => {
      api.setCsrf(csrf);
      await loadMe();
    },
    [loadMe],
  );

  const unlock = useCallback(
    async (pin: string) => {
      await api.post('/api/v1/auth/unlock', { pin }, { noAuthFlow: true });
      await loadMe();
    },
    [loadMe],
  );

  const logout = useCallback(async () => {
    try {
      await api.post('/api/v1/auth/logout', undefined, { noAuthFlow: true });
    } finally {
      api.setCsrf('');
      clearReferenceCache();
      setState({ kind: 'anon', reason: 'logged_out' });
    }
  }, []);

  const value = useMemo<AuthCtx>(() => {
    const me = state.kind === 'ready' ? state.me : null;
    const caps = new Set(me?.capabilities ?? []);
    return {
      state,
      me,
      can: (c) => caps.has(c),
      login,
      afterMfa,
      unlock,
      logout,
      refreshMe: loadMe,
    };
  }, [state, login, afterMfa, unlock, logout, loadMe]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth(): AuthCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error('useAuth outside AuthProvider');
  return v;
}

/** The signed-in user (only call under a ready session). */
export function useMe(): Me {
  const { me } = useAuth();
  if (!me) throw new Error('useMe without a session');
  return me;
}

export function isClerk(me: Me): boolean {
  return me.roles.includes('court_clerk') && !me.capabilities.includes('hearing.outcome');
}
