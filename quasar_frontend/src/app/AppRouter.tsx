import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { DelayedGlobeFallback } from "../components/GlobeSplash";
import { AdminOnly } from "./AdminOnly";
import { ProtectedLayout } from "./ProtectedLayout";
import { ShellLayout } from "./ShellLayout";
import { ClientSetupPage } from "../pages/ClientSetupPage";
import { ConfigSetupPage } from "../pages/ConfigSetupPage";
import { LoginPage } from "../pages/LoginPage";

const DashboardPage = lazy(() =>
  import("../pages/DashboardPage").then((m) => ({ default: m.DashboardPage })),
);
const MonitoringPage = lazy(() =>
  import("../pages/MonitoringPage").then((m) => ({ default: m.MonitoringPage })),
);
const PopsPage = lazy(() => import("../pages/PopsPage").then((m) => ({ default: m.PopsPage })));
const DevicesPage = lazy(() =>
  import("../pages/DevicesPage").then((m) => ({ default: m.DevicesPage })),
);
const CommercialPage = lazy(() =>
  import("../pages/CommercialPage").then((m) => ({ default: m.CommercialPage })),
);
const AlertsPage = lazy(() =>
  import("../pages/AlertsPage").then((m) => ({ default: m.AlertsPage })),
);
const MapPage = lazy(() => import("../pages/MapPage").then((m) => ({ default: m.MapPage })));
const ToolsPage = lazy(() =>
  import("../pages/ToolsPage").then((m) => ({ default: m.ToolsPage })),
);
const SettingsPage = lazy(() =>
  import("../pages/SettingsPage").then((m) => ({ default: m.SettingsPage })),
);
const OltPage = lazy(() => import("../pages/OltPage").then((m) => ({ default: m.OltPage })));
const MikrotikPage = lazy(() =>
  import("../pages/MikrotikPage").then((m) => ({ default: m.MikrotikPage })),
);
const EventsPage = lazy(() =>
  import("../pages/EventsPage").then((m) => ({ default: m.EventsPage })),
);
const IntegrationsHubPage = lazy(() =>
  import("../pages/IntegrationsHubPage").then((m) => ({ default: m.IntegrationsHubPage })),
);
const IntegrationDetailPage = lazy(() =>
  import("../pages/IntegrationDetailPage").then((m) => ({ default: m.IntegrationDetailPage })),
);
const RealtimePage = lazy(() =>
  import("../pages/RealtimePage").then((m) => ({ default: m.RealtimePage })),
);

export function AppRouter() {
  return (
    <Routes>
      <Route path="/client-setup" element={<ClientSetupPage />} />
      <Route path="/config-setup" element={<ConfigSetupPage />} />
      <Route path="/database-setup" element={<Navigate to="/config-setup" replace />} />
      <Route path="/login" element={<LoginPage />} />
      <Route element={<ProtectedLayout />}>
        <Route path="/" element={<ShellLayout />}>
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route
            path="dashboard"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <DashboardPage />
              </Suspense>
            }
          />
          <Route
            path="monitoring"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <MonitoringPage />
              </Suspense>
            }
          />
          <Route
            path="pops"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <PopsPage />
              </Suspense>
            }
          />
          <Route
            path="devices"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <DevicesPage />
              </Suspense>
            }
          />
          <Route
            path="commercial"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <CommercialPage />
              </Suspense>
            }
          />
          <Route
            path="alerts"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <AlertsPage />
              </Suspense>
            }
          />
          <Route
            path="map"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <MapPage />
              </Suspense>
            }
          />
          <Route
            path="tools"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <ToolsPage />
              </Suspense>
            }
          />
          <Route
            path="settings"
            element={
              <AdminOnly>
                <Suspense fallback={<DelayedGlobeFallback />}>
                  <SettingsPage />
                </Suspense>
              </AdminOnly>
            }
          />
          <Route
            path="olt"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <OltPage />
              </Suspense>
            }
          />
          <Route
            path="mikrotik"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <MikrotikPage />
              </Suspense>
            }
          />
          <Route path="bng" element={<Navigate to="/mikrotik" replace />} />
          <Route
            path="events"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <EventsPage />
              </Suspense>
            }
          />
          <Route
            path="integrations"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <IntegrationsHubPage />
              </Suspense>
            }
          />
          <Route
            path="integrations/:slug"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <IntegrationDetailPage />
              </Suspense>
            }
          />
          <Route path="metrics" element={<Navigate to="/integrations" replace />} />
          <Route
            path="realtime"
            element={
              <Suspense fallback={<DelayedGlobeFallback />}>
                <RealtimePage />
              </Suspense>
            }
          />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/login" replace />} />
    </Routes>
  );
}
