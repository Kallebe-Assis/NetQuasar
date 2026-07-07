import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { DelayedGlobeFallback } from "../components/GlobeSplash";
import { NotFoundPage } from "../pages/NotFoundPage";
import { AdminOnly } from "./AdminOnly";
import { IntegrationSlugRedirect } from "./IntegrationSlugRedirect";
import { ProtectedLayout } from "./ProtectedLayout";
import { APP_ROUTES, LEGACY_ROUTE_REDIRECTS } from "./routes";
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
const ClientConnectionsPage = lazy(() =>
  import("../pages/ClientConnectionsPage").then((m) => ({ default: m.ClientConnectionsPage })),
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
const ReportsPage = lazy(() =>
  import("../pages/ReportsPage").then((m) => ({ default: m.ReportsPage })),
);
const EventsPage = lazy(() =>
  import("../pages/EventsPage").then((m) => ({ default: m.EventsPage })),
);
const SwitchPage = lazy(() =>
  import("../pages/SwitchPage").then((m) => ({ default: m.SwitchPage })),
);
const BngPage = lazy(() =>
  import("../pages/BngPage").then((m) => ({ default: m.BngPage })),
);
const IntegrationsHubPage = lazy(() =>
  import("../pages/IntegrationsHubPage").then((m) => ({ default: m.IntegrationsHubPage })),
);
const IntegrationDetailPage = lazy(() =>
  import("../pages/IntegrationDetailPage").then((m) => ({ default: m.IntegrationDetailPage })),
);
const IntegrationConsultPage = lazy(() =>
  import("../pages/IntegrationConsultPage").then((m) => ({ default: m.IntegrationConsultPage })),
);
const RealtimePage = lazy(() =>
  import("../pages/RealtimePage").then((m) => ({ default: m.RealtimePage })),
);

function withSuspense(el: React.ReactNode) {
  return <Suspense fallback={<DelayedGlobeFallback />}>{el}</Suspense>;
}

function legacyRoutePath(from: string) {
  return from.startsWith("/") ? from.slice(1) : from;
}

function legacyRedirects() {
  return Object.entries(LEGACY_ROUTE_REDIRECTS)
    .filter(([from, to]) => from !== "/database-setup" && legacyRoutePath(from) !== legacyRoutePath(to))
    .map(([from, to]) => (
      <Route key={from} path={legacyRoutePath(from)} element={<Navigate to={to} replace />} />
    ));
}

export function AppRouter() {
  return (
    <Routes>
      <Route path={APP_ROUTES.clientSetup} element={<ClientSetupPage />} />
      <Route path={APP_ROUTES.configSetup} element={<ConfigSetupPage />} />
      <Route path="database-setup" element={<Navigate to={APP_ROUTES.configSetup} replace />} />
      <Route path={APP_ROUTES.login} element={<LoginPage />} />

      <Route element={<ProtectedLayout />}>
        {legacyRedirects()}
        <Route path="/" element={<ShellLayout />}>
          <Route index element={<Navigate to={APP_ROUTES.dashboard} replace />} />
          <Route path="dashboard" element={withSuspense(<DashboardPage />)} />
          <Route path="monitoring" element={withSuspense(<MonitoringPage />)} />
          <Route path="realtime" element={withSuspense(<RealtimePage />)} />
          <Route path="integrations" element={withSuspense(<IntegrationsHubPage />)} />
          <Route path="integrations/:slug" element={<IntegrationSlugRedirect />} />
          <Route path="integrations/:slug/consulta" element={withSuspense(<IntegrationConsultPage />)} />
          <Route
            path="integrations/:slug/config"
            element={
              <AdminOnly>
                {withSuspense(<IntegrationDetailPage />)}
              </AdminOnly>
            }
          />
          <Route path="pops" element={withSuspense(<PopsPage />)} />
          <Route path="devices" element={withSuspense(<DevicesPage />)} />
          <Route path="commercial" element={withSuspense(<CommercialPage />)} />
          <Route path="connections" element={withSuspense(<ClientConnectionsPage />)} />
          <Route path="alerts" element={withSuspense(<AlertsPage />)} />
          <Route path="map" element={withSuspense(<MapPage />)} />
          <Route path="tools" element={withSuspense(<ToolsPage />)} />
          <Route
            path="settings"
            element={
              <AdminOnly>
                {withSuspense(<SettingsPage />)}
              </AdminOnly>
            }
          />
          <Route path="olt" element={withSuspense(<OltPage />)} />
          <Route path="mikrotik" element={withSuspense(<MikrotikPage />)} />
          <Route path="switch" element={withSuspense(<SwitchPage />)} />
          <Route path="bng" element={withSuspense(<BngPage />)} />
          <Route path="reports" element={withSuspense(<ReportsPage />)} />
          <Route path="events" element={withSuspense(<EventsPage />)} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to={APP_ROUTES.login} replace />} />
    </Routes>
  );
}
