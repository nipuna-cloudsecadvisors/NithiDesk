import { useEffect, useState } from 'react';
import { api } from './client';
import type { CalendarDay, Court, RefItem, UserView } from './types';

export type RefKind =
  | 'court-levels'
  | 'practice-areas'
  | 'proceeding-types'
  | 'party-role-types'
  | 'hearing-purposes'
  | 'outcome-types'
  | 'relationship-types'
  | 'date-source-types';

// Reference data changes rarely; one fetch per page load is enough. Nothing
// here is persisted (no API data is cached offline, ADR 0008).
const cache = new Map<string, Promise<unknown>>();

function cached<T>(key: string, load: () => Promise<T>): Promise<T> {
  let p = cache.get(key) as Promise<T> | undefined;
  if (!p) {
    p = load();
    cache.set(key, p);
    p.catch(() => cache.delete(key));
  }
  return p;
}

export function clearReferenceCache() {
  cache.clear();
}

export function getReference(kind: RefKind): Promise<RefItem[]> {
  return cached(`ref:${kind}`, async () => {
    const r = await api.get<{ items: RefItem[] | null }>(`/api/v1/reference/${kind}`);
    return (r.items ?? []).slice().sort((a, b) => a.sort_order - b.sort_order);
  });
}

export function getCourts(): Promise<Court[]> {
  return cached('courts', async () => (await api.get<{ items: Court[] | null }>('/api/v1/reference/courts')).items ?? []);
}

export function getUsers(): Promise<UserView[]> {
  return cached('users', async () => (await api.get<{ items: UserView[] | null }>('/api/v1/users')).items ?? []);
}

export function getCalendar(from: string, to: string): Promise<CalendarDay[]> {
  return api
    .get<{ items: CalendarDay[] | null }>('/api/v1/reference/court-calendar', { query: { from, to } })
    .then((r) => r.items ?? []);
}

function useLoaded<T>(load: () => Promise<T>, key: string): T | undefined {
  const [v, setV] = useState<T>();
  useEffect(() => {
    let live = true;
    load()
      .then((x) => live && setV(x))
      .catch(() => undefined);
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  return v;
}

export const useReference = (kind: RefKind) => useLoaded(() => getReference(kind), kind);
export const useCourts = () => useLoaded(getCourts, 'courts');
export const useUsers = () => useLoaded(getUsers, 'users');

/** The authoritative date sources: the only ones a verification may cite. */
export function authoritativeSources(items: RefItem[] | undefined): RefItem[] {
  return (items ?? []).filter((i) => i.attributes?.default_authority === 'authoritative');
}
