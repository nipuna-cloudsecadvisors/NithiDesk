import { lazy, Suspense } from 'react';
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { isClerk, useAuth } from './auth/AuthProvider';
import { StepUpHost } from './auth/StepUp';
import { useT } from './i18n/I18nProvider';
import { LoginScreen, PinUnlockScreen } from './screens/auth/AuthScreens';
import { HearingScreen } from './screens/hearing/HearingScreen';
import { RecordOutcomeScreen } from './screens/hearing/RecordOutcome';
import { CourtListCheckScreen, OutboxScreen, OutcomesPendingScreen, QueueHub, VerificationQueueScreen } from './screens/Queues';
import { MoreScreen, SearchScreen, SettingsScreen } from './screens/Settings';
import { TodayScreen } from './screens/Today';
import { Loading, Notice } from './ui/components';
import { Shell } from './ui/Shell';
import type { Capability } from './api/types';
import type { ComponentType, ReactElement } from 'react';

// Court-day screens (Today, hearing card, outcome, queues) are in the main
// chunk; desk work loads on demand so the phone's first load stays small.
const named = <K extends string>(load: () => Promise<Record<K, ComponentType>>, name: K) =>
  lazy(() => load().then((m) => ({ default: m[name] })));
const MfaScreen = lazy(() => import('./screens/auth/Mfa'));
const WeekScreen = named(() => import('./screens/Week'), 'WeekScreen');
const BriefScreen = named(() => import('./screens/Brief'), 'BriefScreen');
const MattersScreen = named(() => import('./screens/matters/Matters'), 'MattersScreen');
const MatterCreateScreen = named(() => import('./screens/matters/Matters'), 'MatterCreateScreen');
const MatterPage = named(() => import('./screens/matters/MatterPage'), 'MatterPage');
const ImportsScreen = named(() => import('./screens/Imports'), 'ImportsScreen');
const ImportBatchScreen = named(() => import('./screens/Imports'), 'ImportBatchScreen');
const AdminUsersScreen = named(() => import('./screens/Admin'), 'AdminUsersScreen');
const AdminCreateUserScreen = named(() => import('./screens/Admin'), 'AdminCreateUserScreen');
const AdminUserScreen = named(() => import('./screens/Admin'), 'AdminUserScreen');

function RoleHome() {
  const { me } = useAuth();
  return <Navigate to={me && isClerk(me) ? '/verification' : '/today'} replace />;
}

function Need({ cap, children }: { cap: Capability; children: ReactElement }) {
  const { can } = useAuth();
  const { t } = useT();
  return can(cap) ? children : <Notice kind="warn">{t('error.forbidden.capability')}</Notice>;
}

function Routed() {
  const location = useLocation();
  const withBar = /^\/hearings\/[^/]+$/.test(location.pathname);
  return (
    <Shell withActionBar={withBar}>
      <Suspense fallback={<Loading />}>
      <Routes>
        <Route path="/" element={<RoleHome />} />
        <Route path="/today" element={<TodayScreen />} />
        <Route path="/hearings/:id" element={<HearingScreen />} />
        <Route path="/hearings/:id/outcome" element={<RecordOutcomeScreen />} />
        <Route path="/queue" element={<QueueHub />} />
        <Route path="/outbox" element={<OutboxScreen />} />
        <Route path="/verification" element={<VerificationQueueScreen />} />
        <Route
          path="/court-list"
          element={
            <Need cap="diary.court_list_check">
              <CourtListCheckScreen />
            </Need>
          }
        />
        <Route path="/outcomes-pending" element={<OutcomesPendingScreen />} />
        <Route path="/week" element={<WeekScreen />} />
        <Route path="/matters" element={<MattersScreen />} />
        <Route
          path="/matters/new"
          element={
            <Need cap="matter.create">
              <MatterCreateScreen />
            </Need>
          }
        />
        <Route path="/matters/:id" element={<MatterPage />} />
        <Route path="/search" element={<SearchScreen />} />
        <Route path="/brief" element={<BriefScreen />} />
        <Route
          path="/imports"
          element={
            <Need cap="import.run">
              <ImportsScreen />
            </Need>
          }
        />
        <Route
          path="/imports/:id"
          element={
            <Need cap="import.run">
              <ImportBatchScreen />
            </Need>
          }
        />
        <Route
          path="/admin"
          element={
            <Need cap="user.manage">
              <AdminUsersScreen />
            </Need>
          }
        />
        <Route
          path="/admin/users/new"
          element={
            <Need cap="user.manage">
              <AdminCreateUserScreen />
            </Need>
          }
        />
        <Route
          path="/admin/users/:id"
          element={
            <Need cap="user.manage">
              <AdminUserScreen />
            </Need>
          }
        />
        <Route path="/settings" element={<SettingsScreen />} />
        <Route path="/more" element={<MoreScreen />} />
        <Route path="*" element={<RoleHome />} />
      </Routes>
      </Suspense>
    </Shell>
  );
}

export function App() {
  const { state } = useAuth();
  if (state.kind === 'booting') {
    return (
      <div className="auth">
        <div className="auth__card">
          <Loading />
        </div>
      </div>
    );
  }
  if (state.kind === 'anon') return <LoginScreen reason={state.reason} />;
  if (state.kind === 'mfa') {
    return (
      <Suspense fallback={<Loading />}>
        <MfaScreen mode={state.mfa} />
      </Suspense>
    );
  }
  if (state.kind === 'locked') return <PinUnlockScreen />;
  return (
    <BrowserRouter>
      <Routed />
      <StepUpHost />
    </BrowserRouter>
  );
}
