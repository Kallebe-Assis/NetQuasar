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
	rt                *realtimeBroker
	ensureMonitorOnce   sync.Once
	automationONUOnce      sync.Once
	automationReportsOnce  sync.Once
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

// ensureBackgroundSchedulers inicia worker de monitorização e verificações horárias de relatórios (ONU, etc.).
func (s *Server) ensureBackgroundSchedulers() {
	if s.DB() == nil {
		return
	}
	s.ensureMonitoringWorker()
	s.ensureAutomationONUScheduler()
	s.ensureReportSchedulers()
}

func NewServer(log zerolog.Logger, cfg *config.Config, dbHolder *atomic.Pointer[pgxpool.Pool], workerCtx context.Context) http.Handler {
	s := &Server{Log: log, Cfg: cfg, DBHolder: dbHolder, WorkerCtx: workerCtx}
	s.rt = newRealtimeBroker(log, cfg.RedisURL)
	if workerCtx != nil {
		s.rt.Start(workerCtx)
	}
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
			r.Get("/ui-appearance", s.getUIAppearance)
			r.Get("/connection/defaults", s.getConnectionDefaults)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Patch("/monitoring-intervals", s.patchMonitoringIntervals)
				r.Patch("/monitoring", s.patchMonitoringSettings)
				r.Patch("/ui-appearance", s.patchUIAppearance)
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
				r.Get("/olt-vendors/catalog", s.getOltModelsCatalog)
				r.Get("/olt-vendors/{brand}", s.getOltVendor)
				r.Patch("/olt-vendors/{brand}", s.patchOltVendor)
				r.Get("/olt-vendors/{brand}/models", s.listOltVendorModels)
				r.Post("/olt-vendors/{brand}/models", s.createOltVendorModel)
				r.Get("/olt-vendors/{brand}/models/{model}", s.getOltVendorModel)
				r.Patch("/olt-vendors/{brand}/models/{model}", s.patchOltVendorModel)
				r.Delete("/olt-vendors/{brand}/models/{model}", s.deleteOltVendorModel)
				r.Get("/mikrotik-collection", s.getMikrotikCollection)
				r.Patch("/mikrotik-collection", s.patchMikrotikCollection)
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
				r.Get("/automation/alerts-digest", s.getAutomationAlertsDigest)
				r.Patch("/automation/alerts-digest", s.patchAutomationAlertsDigest)
				r.Post("/automation/alerts-digest/run", s.runAutomationAlertsDigest)
				r.Get("/automation/commercial-report", s.getAutomationCommercialReport)
				r.Patch("/automation/commercial-report", s.patchAutomationCommercialReport)
				r.Post("/automation/commercial-report/run", s.runAutomationCommercialReport)
				r.Get("/automation/history", s.getAutomationExecutionHistory)
				r.Get("/notifications/smtp", s.getSMTPSettings)
				r.Patch("/notifications/smtp", s.patchSMTPSettings)
				r.Post("/notifications/smtp/test", s.testSMTPSettings)
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
			r.Get("/{id}/config-backup", s.getDeviceConfigBackup)
			r.Get("/{id}/config-backup/export", s.exportDeviceConfigBackup)
			r.Get("/{id}/status", s.deviceStatusStub)
			r.Get("/{id}", s.getDevice)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/", s.createDevice)
				r.Post("/import/csv", s.importDevicesCSV)
				r.Post("/{id}/checks", s.deviceChecks)
				r.Post("/{id}/telemetry/discover", s.snmpWalkDeviceRun)
				r.Patch("/{id}", s.patchDevice)
				r.Put("/{id}/config-backup", s.putDeviceConfigBackup)
				r.Delete("/{id}", s.deleteDevice)
			})
		})

		r.Route("/commercial", func(r chi.Router) {
			r.Get("/localities", s.listLocalities)
			r.Get("/localities/{id}", s.getLocality)
			r.Get("/monthly-records", s.listMonthlyRecords)
			r.Get("/monthly-records/{id}", s.getMonthlyRecord)
			r.Get("/connections", s.listClientConnections)
			r.Get("/connections/{id}", s.getClientConnection)
			r.Post("/connections/integration-lookup", s.lookupConnectionLoginIntegrations)
			r.Get("/network/fiber-colors", s.listNetworkFiberColors)
			r.Get("/network/projects", s.listNetworkProjects)
			r.Get("/network/projects/{id}", s.getNetworkProject)
			r.Get("/network/ctos", s.listNetworkCtos)
			r.Get("/network/ctos/{id}", s.getNetworkCto)
			r.Get("/network/splice-boxes", s.listNetworkSpliceBoxes)
			r.Get("/network/splice-boxes/{id}", s.getNetworkSpliceBox)
			r.Get("/network/cables", s.listNetworkCables)
			r.Get("/network/poles", s.listNetworkPoles)
			r.Get("/aggregates", s.commercialAggregates)
			r.Get("/totals-history", s.commercialTotalsHistory)
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
				r.Post("/connections/check-duplicates", s.checkClientConnectionDuplicates)
				r.Post("/connections", s.createClientConnection)
				r.Post("/connections/bulk", s.bulkClientConnections)
				r.Post("/connections/import/csv", s.importClientConnectionsCSV)
				r.Patch("/connections/{id}", s.patchClientConnection)
				r.Delete("/connections/{id}", s.deleteClientConnection)
				r.Post("/network/projects", s.createNetworkProject)
				r.Patch("/network/projects/{id}", s.patchNetworkProject)
				r.Delete("/network/projects/{id}", s.deleteNetworkProject)
				r.Post("/network/ctos", s.createNetworkCto)
				r.Patch("/network/ctos/{id}", s.patchNetworkCto)
				r.Delete("/network/ctos/{id}", s.deleteNetworkCto)
				r.Post("/network/splice-boxes", s.createNetworkSpliceBox)
				r.Patch("/network/splice-boxes/{id}", s.patchNetworkSpliceBox)
				r.Delete("/network/splice-boxes/{id}", s.deleteNetworkSpliceBox)
				r.Post("/network/cables", s.createNetworkCable)
				r.Patch("/network/cables/{id}", s.patchNetworkCable)
				r.Delete("/network/cables/{id}", s.deleteNetworkCable)
				r.Post("/network/poles", s.createNetworkPole)
				r.Patch("/network/poles/{id}", s.patchNetworkPole)
				r.Delete("/network/poles/{id}", s.deleteNetworkPole)
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
			r.Get("/ignored", s.alertsIgnoredList)
			r.Get("/incidents/active", s.incidentsActive)
			r.Get("/incidents/{id}", s.incidentDetail)
			r.Get("/suppressions", s.listSuppressions)
			r.Get("/suppressions/{id}", s.getSuppression)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/incidents/reconcile", s.incidentsReconcile)
				r.Post("/revalidate", s.alertsRevalidate)
				r.Post("/verify-all", s.alertsVerifyAll)
				r.Post("/ignored/{id}/reactivate", s.alertIgnoreReactivate)
				r.Post("/{id}/ignore", s.alertIgnore)
				r.Post("/{id}/verify", s.alertVerifyOne)
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

		r.Route("/integrations", func(r chi.Router) {
			r.Get("/", s.listIntegrations)
			r.Get("/{id}", s.getIntegration)
			r.Get("/{id}/consumer", s.getIntegrationConsumerMeta)
			r.Post("/{id}/consumer/client-search", s.integrationConsumerClientSearch)
			r.Post("/{id}/consumer/client-attendance", s.integrationConsumerClientAttendance)
			r.Post("/{id}/consumer/client-work-order", s.integrationConsumerClientWorkOrder)
			r.Post("/{id}/consumer/client-login", s.integrationConsumerClientLogin)
			r.Get("/{id}/logs", s.listIntegrationLogs)
			r.Post("/{id}/test", s.integrationTest)
			r.Post("/{id}/login", s.integrationLogin)
			r.Post("/{id}/run-all", s.integrationRunAll)
			r.Post("/{id}/requests/{requestId}/run", s.integrationRunRequest)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/", s.createIntegration)
				r.Patch("/{id}", s.patchIntegration)
				r.Delete("/{id}", s.deleteIntegration)
				r.Post("/{id}/requests", s.createIntegrationRequest)
				r.Patch("/{id}/requests/{requestId}", s.patchIntegrationRequest)
				r.Delete("/{id}/requests/{requestId}", s.deleteIntegrationRequest)
			})
		})

		r.Route("/tools", func(r chi.Router) {
			r.Post("/dns/run", s.toolsDNSRun)
			r.Post("/http-https-probe", s.toolsHTTPProbeStub)
			r.Post("/icmp/ping", s.toolsICMPPing)
			r.Post("/tracert", s.toolsTracert)
			r.Post("/nmap", s.toolsNmap)
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
			r.Get("/devices/{id}/snmp-debug", s.getOLTSnmpDebug)
			r.Get("/reports/history", s.getOLTReportsHistory)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/devices/{id}/refresh", s.refreshOLTDevice)
				r.Post("/devices/{id}/snmp-debug", s.postOLTSnmpDebug)
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
		r.Get("/realtime/ws", s.realtimeWS)
		r.Get("/events", s.listEvents)
		r.Get("/metrics", s.metricsSeries)

		r.Route("/reports", func(r chi.Router) {
			r.Get("/system", s.systemReportsCatalog)
			r.Get("/system/{id}", s.systemReportData)
			r.Get("/system/{id}/csv", s.systemReportCSV)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdminMiddleware)
				r.Post("/system/{id}/telegram", s.systemReportTelegram)
			})
		})

		r.Get("/map/equipment-points/{deviceId}", s.mapEquipmentPointDetail)
		r.Get("/map/equipment-points", s.mapEquipmentPoints)
		r.Get("/map/connection-points", s.mapConnectionPoints)

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
		s.ensureBackgroundSchedulers()
	}

	return chain(cfg, log, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "netquasar-backend"})
}
