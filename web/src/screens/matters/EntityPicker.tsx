import { useEffect, useState } from 'react';
import { api } from '../../api/client';
import type { Entity, EntityInput } from '../../api/types';
import { useT } from '../../i18n/I18nProvider';
import { Field } from '../../ui/components';

export type EntityChoice = { entity_id: string; label: string } | { new: EntityInput };

const KINDS = ['person', 'organisation', 'government_body'] as const;

/** Search the registry (cross-script) or create a new person/organisation. */
export function EntityPicker({
  label,
  value,
  onChange,
  error,
}: {
  label: string;
  value: EntityChoice | null;
  onChange: (v: EntityChoice | null) => void;
  error?: string;
}) {
  const { t, tl, name, otherName } = useT();
  const [q, setQ] = useState('');
  const [hits, setHits] = useState<Entity[]>([]);
  const [creating, setCreating] = useState(false);
  const [draft, setDraft] = useState<EntityInput>({ kind: 'person', name_en: '', name_si: '' });

  useEffect(() => {
    const term = q.trim();
    if (term.length < 2) {
      setHits([]);
      return;
    }
    const ctl = new AbortController();
    const timer = window.setTimeout(() => {
      api
        .get<{ items: Entity[] | null }>('/api/v1/entities', { query: { q: term, limit: 10 }, signal: ctl.signal })
        .then((r) => setHits(r.items ?? []))
        .catch(() => undefined);
    }, 250);
    return () => {
      ctl.abort();
      window.clearTimeout(timer);
    };
  }, [q]);

  useEffect(() => {
    if (creating && (draft.name_en.trim() || draft.name_si.trim())) onChange({ new: draft });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [creating, draft]);

  if (value && 'entity_id' in value) {
    return (
      <div className="field">
        <span className="field__label">{label}</span>
        <div className="row panel">
          <span className="grow">{value.label}</span>
          <button type="button" className="btn btn--small" onClick={() => onChange(null)}>
            {t('common.change')}
          </button>
        </div>
      </div>
    );
  }

  if (creating) {
    return (
      <fieldset className="field panel">
        <legend>{label}</legend>
        <div className="stack">
          <Field label={tl('entity.kind')}>
            {(p) => (
              <select {...p} value={draft.kind} onChange={(e) => setDraft({ ...draft, kind: e.target.value })}>
                {KINDS.map((k) => (
                  <option key={k} value={k}>
                    {t(`entity.kind.${k}`)}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <div className="grid-2">
            <Field label={tl('entity.nameEn')} error={error}>
              {(p) => <input {...p} type="text" lang="en" value={draft.name_en} onChange={(e) => setDraft({ ...draft, name_en: e.target.value })} />}
            </Field>
            <Field label={tl('entity.nameSi')}>
              {(p) => <input {...p} type="text" lang="si" value={draft.name_si} onChange={(e) => setDraft({ ...draft, name_si: e.target.value })} />}
            </Field>
          </div>
          <div className="row">
            <button
              type="button"
              className="btn btn--quiet btn--small"
              onClick={() => {
                setCreating(false);
                onChange(null);
              }}
            >
              {t('entity.searchInstead')}
            </button>
          </div>
        </div>
      </fieldset>
    );
  }

  return (
    <div className="stack stack--tight">
      <Field label={label} hint={t('entity.searchHint')} error={error}>
        {(p) => <input {...p} type="search" value={q} onChange={(e) => setQ(e.target.value)} autoComplete="off" />}
      </Field>
      {hits.length > 0 ? (
        <ul className="list">
          {hits.map((h) => (
            <li key={h.id}>
              <button
                type="button"
                className="btn btn--quiet btn--block menu"
                onClick={() => onChange({ entity_id: h.id, label: [name(h.name), otherName(h.name)].filter(Boolean).join(' / ') })}
              >
                <span>
                  {name(h.name)}
                  {otherName(h.name) ? <span className="muted">{` / ${otherName(h.name)}`}</span> : null}
                  {h.match ? <span className="tag">{` ${t(`entity.match.${h.match}`)}`}</span> : null}
                </span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <div>
        <button
          type="button"
          className="btn btn--small"
          onClick={() => {
            setCreating(true);
            setDraft((d) => ({ ...d, name_en: /[඀-෿]/.test(q) ? d.name_en : q, name_si: /[඀-෿]/.test(q) ? q : d.name_si }));
          }}
        >
          {t('entity.createNew')}
        </button>
      </div>
    </div>
  );
}

export function entityPayload(c: EntityChoice): { entity_id: string } | { new: EntityInput } {
  if ('entity_id' in c) return { entity_id: c.entity_id };
  return { new: { kind: c.new.kind, name_en: c.new.name_en.trim(), name_si: c.new.name_si.trim() } };
}
