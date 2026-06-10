import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { DelayedGlobeFallback } from "../components/GlobeSplash";
import { NotFoundPage } from "../pages/NotFoundPage";
import { AdminOnly } from "./AdminOnly";
import { IntegrationSlugRedirect } from "./IntegrationSlugRedirect";
import { ProtectedLayout } from "./ProtectedLayout";
import { APP_ROUTES } from "./routes";
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
const EventsPage = lazy(() =>
  import("../pages/EventsPage").then((m) => ({ default: m.EventsPage })),
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

export function AppRouter() {
  return (
    <Routes>
      <Route path={APP_ROUTES.clientSetup} element={<ClientSetupPage />} />
      <Route path={APP_ROUTES.configSetup} element={<ConfigSetupPage />} />
      <Route path="database-setup" element={<Navigate to={APP_ROUTES.configSetup} replace />} />
      <Route path={APP_ROUTES.login} element={<LoginPage />} />

      <Route element={<ProtectedLayout />}>
        <Route path="monitoramento" element={<Navigate to={APP_ROUTES.monitoring} replace />} />
        <Route path="equipamentos" element={<Navigate to={APP_ROUTES.devices} replace />} />
        <Route path="configuracoes" element={<Navigate to={APP_ROUTES.settings} replace />} />
        <Route path="ferramentas" element={<Navigate to={APP_ROUTES.tools} replace />} />
        <Route path="alertas" element={<Navigate to={APP_ROUTES.alerts} replace />} />
        <Route path="mapa" element={<Navigate to={APP_ROUTES.map} replace />} />
        <Route path="comercial" element={<Navigate to={APP_ROUTES.commercial} replace />} />
        <Route path="conexoes" element={<Navigate to={APP_ROUTES.connections} replace />} />
        <Route path="integracoes" element={<Navigate to={APP_ROUTES.integrations} replace />} />
        <Route path="tempo-real" element={<Navigate to={APP_ROUTES.realtime} replace />} />
        <Route path="eventos" element={<Navigate to={APP_ROUTES.events} replace />} />
        <Route path="overview" element={<Navigate to={APP_ROUTES.dashboard} replace />} />
        <Route path="bng" element={<Navigate to={APP_ROUTES.mikrotik} replace />} />
        <Route path="metrics" element={<Navigate to={APP_ROUTES.integrations} replace />} />
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
          <Route path="events" element={withSuspense(<EventsPage />)} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to={APP_ROUTES.login} replace />} />
    </Routes>
  );
}
