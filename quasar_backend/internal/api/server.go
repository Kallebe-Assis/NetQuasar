package api

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/embedui"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/rs/zerolog"
)

// Server agrupa dependências HTTP.
type Server struct {
	Log               zerolog.Logger
	Cfg               *config.Config
	DBHolder          *atomic.Pointer[pgxpool.Pool] // pool atual; trocável em runtime (PATCH /settings/database)
	WorkerCtx         context.Context               // cancelado no shutdown; nil desativa o worker de monitorização
	ensureMonitorOnce sync.Once
}

// DB retorna o pool PostgreSQL ativo ou nil (testes sem holder).
func (s *Server) DB() *pgxpool.Pool {
	if s.DBHolder == nil {
		return nil
	}
	return s.DBHolder.Load()
}

func (s *Server) ensureMonitoringWorker() {
	if s.WorkerCtx == nil || s.DBHolder == nil {
		return
	}
	s.ensureMonitorOnce.Do(func() {
		go monitorworker.Run(s.WorkerCtx, s.DBHolder, s.Log)
	})
}

func NewServer(log zerolog.Logger, cfg *config.Config, dbHolder *atomic.Pointer[pgxpool.Pool], workerCtx context.Context) http.Handler {
	s := &Server{Log: log, Cfg: cfg, DBHolder: dbHolder, WorkerCtx: workerCtx}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	if len(cfg.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   cfg.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
			AllowCredentials: true,
		}))
	}

	r.Get("/health", s.health)
	r.Handle("/metrics", http.HandlerFunc(s.prometheusMetrics))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.health)
		r.Get("/setup/status", s.setupStatus)
		r.Post("/setup/database/test", s.setupDatabaseTest)
		r.Post("/setup/database/apply", s.setupDatabaseApply)
		r.Post("/auth/login", s.authLogin)

		r.Route("/monitoring", func(r chi.Router) {
			r.Get("/internet-check", s.internetCheck)
			r.Get("/state", s.monitoringState)
			r.Get("/cycles/kinds", s.monitoringCycleKinds)
			r.Get("/active-equipment", s.monitoringActiveEquipment)
			r.Get("/nightly-collection", s.getNightlyCollectionSettings)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/start", s.monitoringStart)
				r.Post("/stop", s.monitoringStop)
				r.Post("/reload-devices", s.monitoringReloadDevices)
				r.Post("/cycles/{cycle}", s.monitoringCycleRun)
				r.Patch("/nightly-collection", s.patchNightlyCollectionSettings)
				r.Post("/nightly-collection/run", s.runNightlyCollectionNow)
				r.Post("/full-report/devices/{id}", s.monitoringFullReportDevice)
			})
		})

		r.Route("/settings", func(r chi.Router) {
			r.Get("/monitoring-intervals", s.getMonitoringIntervals)
			r.Get("/monitoring", s.getMonitoringSettings)
			r.Get("/connection/defaults", s.getConnectionDefaults)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Patch("/monitoring-intervals", s.patchMonitoringIntervals)
				r.Patch("/monitoring", s.patchMonitoringSettings)
				r.Get("/database", s.getDatabaseMeta)
				r.Patch("/database", s.patchDatabaseMeta)
				r.Post("/database/test", s.testDatabaseConnection)
				r.Get("/database/logs", s.settingsDatabaseLogs)
				r.Get("/users", s.listUsers)
				r.Post("/users", s.createUser)
				r.Get("/users/{id}", s.getUser)
				r.Patch("/users/{id}", s.patchUser)
				r.Delete("/users/{id}", s.deleteUser)
				r.Patch("/connection/defaults", s.patchConnectionDefaults)
				r.Get("/olt-vendors", s.listOltVendors)
				r.Get("/olt-vendors/{brand}", s.getOltVendor)
				r.Patch("/olt-vendors/{brand}", s.patchOltVendor)
				r.Get("/notifications/telegram/monitoring", s.getTelegramMonitoring)
				r.Patch("/notifications/telegram/monitoring", s.patchTelegramMonitoring)
				r.Post("/notifications/telegram/monitoring/test", s.testTelegramMonitoring)
				r.Get("/notifications/telegram/reports", s.getTelegramReports)
				r.Patch("/notifications/telegram/reports", s.patchTelegramReports)
				r.Post("/notifications/telegram/reports/test", s.testTelegramReports)
				r.Get("/automation/onu-monthly-report", s.getAutomationONU)
				r.Patch("/automation/onu-monthly-report", s.patchAutomationONU)
				r.Post("/automation/onu-monthly-report/run", s.runAutomationONU)
				r.Get("/automation/onu-monthly-report/runs", s.listAutomationRuns)
			})
		})

		r.Route("/pops", func(r chi.Router) {
			r.Get("/", s.listPops)
			r.Get("/{id}/contacts", s.listPopContacts)
			r.Get("/{id}", s.getPop)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/", s.createPop)
				r.Patch("/{id}", s.patchPop)
				r.Delete("/{id}", s.deletePop)
				r.Post("/{id}/devices/bulk", s.bulkAttachDevices)
				r.Post("/{id}/contacts", s.createPopContact)
				r.Delete("/contacts/{contactId}", s.deletePopContact)
			})
		})

		r.Route("/devices", func(r chi.Router) {
			r.Get("/", s.listDevices)
			r.Get("/export", s.devicesExport)
			r.Get("/{id}/snmp-inventory", s.getDeviceSNMPInventory)
			r.Get("/{id}/status", s.deviceStatusStub)
			r.Get("/{id}", s.getDevice)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/", s.createDevice)
				r.Post("/import/csv", s.importDevicesCSV)
				r.Post("/{id}/checks", s.deviceChecks)
				r.Post("/{id}/telemetry/discover", s.snmpWalkDeviceRun)
				r.Patch("/{id}", s.patchDevice)
				r.Delete("/{id}", s.deleteDevice)
			})
		})

		r.Route("/commercial", func(r chi.Router) {
			r.Get("/localities", s.listLocalities)
			r.Get("/localities/{id}", s.getLocality)
			r.Get("/monthly-records", s.listMonthlyRecords)
			r.Get("/monthly-records/{id}", s.getMonthlyRecord)
			r.Get("/aggregates", s.commercialAggregates)
			r.Get("/comparison", s.commercialMonthComparison)
			r.Get("/reports/export", s.commercialReportsExport)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/localities", s.createLocality)
				r.Patch("/localities/{id}", s.patchLocality)
				r.Delete("/localities/{id}", s.deleteLocality)
				r.Post("/monthly-records", s.createMonthlyRecord)
				r.Post("/monthly-records/bulk", s.bulkMonthlyRecords)
				r.Patch("/monthly-records/{id}", s.patchMonthlyRecord)
				r.Delete("/monthly-records/{id}", s.deleteMonthlyRecord)
				r.Post("/reports/send-telegram", s.commercialReportsSendTelegram)
			})
		})
		r.Route("/maintenance", func(r chi.Router) {
			r.Get("/windows", s.listMaintenanceWindows)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/windows", s.createMaintenanceWindow)
				r.Patch("/windows/{id}", s.patchMaintenanceWindow)
			})
		})
		r.Route("/ops", func(r chi.Router) {
			r.Get("/audit", s.listOpsAudit)
		})

		r.Route("/alerts", func(r chi.Router) {
			r.Get("/active", s.alertsActive)
			r.Get("/history", s.alertsHistory)
			r.Get("/suppressions", s.listSuppressions)
			r.Get("/suppressions/{id}", s.getSuppression)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/revalidate", s.alertsRevalidate)
				r.Post("/suppressions", s.createSuppression)
				r.Patch("/suppressions/{id}", s.patchSuppression)
				r.Delete("/suppressions/{id}", s.deleteSuppression)
			})
		})

		r.Route("/alert-rules", func(r chi.Router) {
			r.Get("/", s.listAlertRules)
			r.Get("/{id}", s.getAlertRule)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/", s.createAlertRule)
				r.Patch("/{id}", s.patchAlertRule)
				r.Delete("/{id}", s.deleteAlertRule)
				r.Post("/{id}/test", s.testAlertRule)
			})
		})

		r.Route("/tools", func(r chi.Router) {
			r.Post("/dns/run", s.toolsDNSRun)
			r.Post("/http-https-probe", s.toolsHTTPProbeStub)
			r.Post("/icmp/ping", s.toolsICMPPing)
			r.Post("/snmp/get", s.toolsSNMPGet)
			r.Post("/snmp/bulk-get", s.toolsSNMPBulkGet)
			r.Post("/telnet/test", s.toolsTelnetTest)
			r.Post("/ssh/test", s.toolsSSHTest)
			r.Post("/snmp-walk/run", s.toolsSNMPWalkRun)
			r.Get("/snmp-walk/jobs/{jobId}/rows", s.toolsSNMPWalkJobRows)
			r.Get("/snmp-walk/jobs/{jobId}/discoveries", s.toolsSNMPWalkJobDiscoveries)
			r.Post("/mikrotik/quick-metrics", s.toolsMikrotikQuickMetrics)
			r.Post("/mikrotik/interfaces", s.toolsMikrotikInterfaces)
			r.Post("/mikrotik/walk", s.toolsMikrotikWalk)
		})

		r.Route("/ping", func(r chi.Router) {
			r.Get("/devices/{id}/latest", s.pingLatest)
			r.Get("/history", s.pingHistory)
			r.Get("/devices/{id}/run", s.pingRunStub)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/devices/{id}/run", s.pingRunStub)
			})
		})

		r.Route("/telemetry", func(r chi.Router) {
			r.Get("/devices/{id}/latest", s.telemetryLatest)
			r.Get("/history", s.telemetryHistory)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/devices/{id}/collect", s.telemetryCollect)
			})
		})

		r.Route("/interfaces", func(r chi.Router) {
			r.Get("/devices/{id}", s.listDeviceInterfaces)
			r.Get("/history", s.interfacesHistory)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/devices/{id}/refresh", s.refreshDeviceInterfaces)
				r.Post("/devices/{id}/realtime", s.realtimeDeviceInterfaces)
			})
		})

		r.Route("/olt", func(r chi.Router) {
			r.Get("/devices", s.listOLTDevices)
			r.Get("/devices/{id}", s.getOLTDevice)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/devices/{id}/refresh", s.refreshOLTDevice)
			})
		})

		r.Route("/snmp-walk", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/devices/{id}/run", s.snmpWalkDeviceRun)
				r.Get("/devices/{id}/jobs/{jobId}", s.snmpWalkJobGet)
				r.Get("/devices/{id}/candidates", s.snmpWalkCandidates)
			})
		})

		r.Route("/bng", func(r chi.Router) {
			r.Get("/sessions", s.bngSessions)
			r.Get("/sessions/search", s.bngSessionsSearch)
			r.Get("/auth/logs", s.bngAuthLogs)
			r.Get("/traffic/users", s.bngTrafficUsers)
			r.Get("/stats/summary", s.bngStatsSummary)
		})

		r.Get("/realtime/ping", s.realtimePing)
		r.Get("/events", s.listEvents)
		r.Get("/metrics", s.metricsSeries)

		r.Get("/map/equipment-points/{deviceId}", s.mapEquipmentPointDetail)
		r.Get("/map/equipment-points", s.mapEquipmentPoints)

		r.Get("/overview/summary", s.overviewSummaryStub)
		r.Get("/overview/top-latency", s.overviewTopLatency)
		r.Get("/dashboard/analytics", s.dashboardAnalytics)
		r.Get("/dashboard/data-gaps", s.dashboardDataGaps)
		r.Get("/dashboard/olt-capacity", s.dashboardOltCapacity)
	})

	if cfg.EmbeddedUI {
		r.Handle("/*", embedui.Handler(log))
	}

	if s.DB() != nil {
		s.ensureMonitoringWorker()
	}

	return chain(cfg, log, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "netquasar-backend"})
}
