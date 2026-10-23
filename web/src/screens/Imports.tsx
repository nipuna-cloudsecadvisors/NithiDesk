import { useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../api/client';
import type { ImportBatch, ImportRow } from '../api/types';
import { useT } from '../i18n/I18nProvider';
import { uuidv4 } from '../lib/uuid';
import { Empty, ErrorNotice, Field, Loading, Notice, Segmented, useAsync, useToast } from '../ui/components';

export function ImportsScreen() {
  const { t, tl, fieldError } = useT();
  const navigate = useNavigate();
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const upload = async (e: FormEvent) => {
    e.preventDefault();
    if (!file) return;
    setBusy(true);
    setError(null);
    try {
      const form = new FormData();
      form.set('kind', 'diary_csv');
      form.set('file', file, file.name);
      const batch = await api.request<ImportBatch>('POST', '/api/v1/imports', { form });
      navigate(`/imports/${batch.id}`);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <h1>{tl('nav.imports')}</h1>
          <p>{t('import.explain')}</p>
        </div>
      </div>
      <ol className="stack">
        <li className="panel stack">
          <h2>{t('import.step1')}</h2>
          <p>{t('import.templateExplain')}</p>
          <p>
            <a className="btn" href="/api/v1/imports/template.csv" download>
              {t('import.downloadTemplate')}
            </a>
          </p>
        </li>
        <li className="panel">
          <form className="form" onSubmit={upload}>
            <h2>{t('import.step2')}</h2>
            <ErrorNotice error={error} />
            <Field label={tl('import.file')} hint={t('import.fileHint')} error={fieldError(error, 'file')}>
              {(p) => <input {...p} type="file" accept=".csv,text/csv" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />}
            </Field>
            <div>
              <button type="submit" className="btn btn--primary" disabled={busy || !file}>
                {busy ? t('import.uploading') : t('import.upload')}
              </button>
            </div>
          </form>
        </li>
      </ol>
    </>
  );
}

type RowFilter = 'error' | 'warning' | 'ok';

export function ImportBatchScreen() {
  const { id = '' } = useParams();
  const { t, tl, has } = useT();
  const toast = useToast();
  const batch = useAsync((signal) => api.get<ImportBatch>(`/api/v1/imports/${id}`, { signal }), [id]);
  const [filter, setFilter] = useState<RowFilter>('error');
  const rows = useAsync(
    (signal) =>
      api.get<{ items: ImportRow[] | null }>(`/api/v1/imports/${id}/rows`, { query: { status: filter }, signal }).then((r) => r.items ?? []),
    [id, filter, batch.data?.status],
  );
  const commitKey = useMemo(() => uuidv4(), []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const act = async (fn: () => Promise<unknown>, done: string) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      toast('ok', done);
      batch.reload();
    } catch (err) {
      setError(err);
      batch.reload();
    } finally {
      setBusy(false);
    }
  };

  if (batch.loading && !batch.data) return <Loading />;
  if (!batch.data) return <ErrorNotice error={batch.error} />;
  const b = batch.data;
  const columns = Array.from(new Set((rows.data ?? []).flatMap((r) => Object.keys(r.raw ?? {})))).slice(0, 8);
  const message = (m: { code: string; field?: string; params?: Record<string, unknown> }) => {
    const key = `importcode.${m.code}`;
    const fieldKey = `fieldcode.${m.code}`;
    const text = has(key) ? t(key) : has(fieldKey) ? t(fieldKey) : t('import.unknownCode', { code: m.code });
    return m.field ? t('import.fieldMessage', { field: m.field, message: text }) : text;
  };

  return (
    <>
      <div className="page-head">
        <div className="page-head__title">
          <p>
            <Link to="/imports">{t('import.back')}</Link>
          </p>
          <h1>{t('import.batchTitle')}</h1>
          <p>{t(`importStatus.${b.status}`)}</p>
        </div>
      </div>
      <ErrorNotice error={error} />
      <div className="highlights">
        <div className="highlight">
          <strong>{b.row_count}</strong>
          {t('import.rows')}
        </div>
        <div className={b.ok_count ? 'highlight' : 'highlight'}>
          <strong>{b.ok_count}</strong>
          {t('import.ok')}
        </div>
        <div className={b.warning_count ? 'highlight highlight--warm' : 'highlight'}>
          <strong>{b.warning_count}</strong>
          {t('import.warnings')}
        </div>
        <div className={b.error_count ? 'highlight highlight--hot' : 'highlight'}>
          <strong>{b.error_count}</strong>
          {t('import.errors')}
        </div>
      </div>

      <div className="row">
        {b.status === 'uploaded' || b.status === 'validated' ? (
          <button type="button" className="btn" disabled={busy} onClick={() => void act(() => api.post(`/api/v1/imports/${id}/validate`), t('import.validated'))}>
            {b.status === 'uploaded' ? t('import.validate') : t('import.revalidate')}
          </button>
        ) : null}
        {b.status === 'validated' ? (
          <button
            type="button"
            className="btn btn--primary"
            disabled={busy || b.error_count > 0}
            onClick={() =>
              void act(
                () =>
                  api.post(`/api/v1/imports/${id}/commit`, undefined, {
                    idempotencyKey: commitKey,
                    ifMatch: b.row_version ?? 0,
                  }),
                t('import.committed'),
              )
            }
          >
            {t('import.commit', { count: b.ok_count + b.warning_count })}
          </button>
        ) : null}
        {b.status === 'committed' ? (
          <button
            type="button"
            className="btn btn--danger"
            disabled={busy}
            onClick={() => {
              if (window.confirm(t('import.rollbackConfirm'))) void act(() => api.post(`/api/v1/imports/${id}/rollback`), t('import.rolledBack'));
            }}
          >
            {t('import.rollback')}
          </button>
        ) : null}
      </div>
      {b.status === 'validated' && b.error_count > 0 ? (
        <div className="section">
          <Notice kind="warn">{t('import.fixErrors')}</Notice>
        </div>
      ) : null}

      <section className="section">
        <div className="row">
          <h2 className="grow">{tl('import.report')}</h2>
          <Segmented<RowFilter>
            label={t('import.filter')}
            value={filter}
            onChange={setFilter}
            options={[
              { value: 'error', label: t('import.errors') },
              { value: 'warning', label: t('import.warnings') },
              { value: 'ok', label: t('import.ok') },
            ]}
          />
        </div>
        {rows.loading && !rows.data ? <Loading /> : null}
        <ErrorNotice error={rows.error} />
        {rows.data && rows.data.length === 0 ? <Empty>{t('import.noRows')}</Empty> : null}
        {rows.data && rows.data.length > 0 ? (
          <div className="table-wrap section">
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">{t('import.rowNumber')}</th>
                  <th scope="col">{t('import.messages')}</th>
                  {columns.map((c) => (
                    <th scope="col" key={c}>
                      {c}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.data.map((r) => (
                  <tr key={r.row_number}>
                    <td>{r.row_number}</td>
                    <td>
                      {(r.messages ?? []).map((m, i) => (
                        <p key={i} className={r.status === 'error' ? 'field__error' : undefined}>
                          {message(m)}
                        </p>
                      ))}
                    </td>
                    {columns.map((c) => (
                      <td key={c}>{r.raw?.[c] ?? ''}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </section>
    </>
  );
}
