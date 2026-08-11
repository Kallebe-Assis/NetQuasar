package api

import (
	"context"
	"net/http"
	"strings"
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
	ensureMonitorOnce      sync.Once
	automationONUOnce     sync.Once
	automationReportsOnce sync.Once
	// sysCfgImportMu protege o mapa de jobs de importação de configuração (aba Base de dados).
	sysCfgImportMu   sync.Mutex
	sysCfgImportJobs map[string]*sysConfigImportJob
	dbRestoreMu      sync.Mutex
	dbRestoreJobs    map[string]*dbRestoreJob
	bngCollectProgress *bngCollectProgressStore
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
	s := &Server{
		Log:                log,
		Cfg:                cfg,
		DBHolder:           dbHolder,
		WorkerCtx:          workerCtx,
		sysCfgImportJobs:   make(map[string]*sysConfigImportJob),
		dbRestoreJobs:      make(map[string]*dbRestoreJob),
		bngCollectProgress: newBngCollectProgressStore(),
	}
	if workerCtx != nil {
		registerOltManualRefresher(s)
	}
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
			r.Get("/olt-collect-readiness", s.monitoringOltCollectReadiness)
			r.Get("/nightly-collection", s.getNightlyCollectionSettings)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("monitoring.control", "*"))
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
				r.Use(s.requirePermissionMiddleware(
					"settings.system", "settings.monitoring", "settings.notifications",
					"settings.users", "settings.permissions", "*",
				))
				r.Patch("/monitoring-intervals", s.patchMonitoringIntervals)
				r.Patch("/monitoring", s.patchMonitoringSettings)
				r.Patch("/ui-appearance", s.patchUIAppearance)
				r.Get("/database", s.getDatabaseMeta)
				r.Patch("/database", s.patchDatabaseMeta)
				r.Post("/database/test", s.testDatabaseConnection)
				r.Get("/database/cleanup/overview", s.databaseCleanupOverview)
				r.Post("/database/cleanup/scan", s.databaseCleanupScan)
				r.Post("/database/cleanup/execute", s.databaseCleanupExecute)
				r.Get("/database/logs", s.settingsDatabaseLogs)
				r.Get("/database/backups/b2", s.listDatabaseBackupsB2)
				r.Post("/database/backups/upload", s.uploadDatabaseBackupRestore)
				r.Post("/database/backups/restore", s.restoreDatabaseBackup)
				r.Get("/database/backups/restore/{jobId}", s.getDatabaseRestoreJob)
				r.Get("/system-config/export", s.exportSystemConfiguration)
				r.Post("/system-config/import", s.startSystemConfigurationImport)
				r.Get("/system-config/import/{jobId}", s.getSystemConfigurationImportJob)
				r.Get("/users", s.listUsers)
				r.Post("/users", s.createUser)
				r.Get("/users/{id}", s.getUser)
				r.Patch("/users/{id}", s.patchUser)
				r.Delete("/users/{id}", s.deleteUser)
				r.Get("/permissions", s.listPermissionCatalog)
				r.Get("/permission-profiles", s.listPermissionProfiles)
				r.Post("/permission-profiles", s.createPermissionProfile)
				r.Get("/permission-profiles/{id}", s.getPermissionProfile)
				r.Patch("/permission-profiles/{id}", s.patchPermissionProfile)
				r.Delete("/permission-profiles/{id}", s.deletePermissionProfile)
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
				r.Get("/mikrotik-telnet-profiles", s.listMikrotikTelnetProfiles)
				r.Post("/mikrotik-telnet-profiles", s.createMikrotikTelnetProfile)
				r.Get("/mikrotik-telnet-profiles/{id}", s.getMikrotikTelnetProfile)
				r.Patch("/mikrotik-telnet-profiles/{id}", s.patchMikrotikTelnetProfile)
				r.Delete("/mikrotik-telnet-profiles/{id}", s.deleteMikrotikTelnetProfile)
				r.Get("/switch-collection", s.getSwitchCollection)
				r.Patch("/switch-collection", s.patchSwitchCollection)
				r.Get("/switch-telnet-profiles", s.listSwitchTelnetProfiles)
				r.Post("/switch-telnet-profiles", s.createSwitchTelnetProfile)
				r.Get("/switch-telnet-profiles/{id}", s.getSwitchTelnetProfile)
				r.Patch("/switch-telnet-profiles/{id}", s.patchSwitchTelnetProfile)
				r.Delete("/switch-telnet-profiles/{id}", s.deleteSwitchTelnetProfile)
				r.Get("/bng-collection", s.getBngCollection)
				r.Patch("/bng-collection", s.patchBngCollection)
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
				r.Get("/automation/bng-stats-report", s.getAutomationBngStatsReport)
				r.Patch("/automation/bng-stats-report", s.patchAutomationBngStatsReport)
				r.Post("/automation/bng-stats-report/run", s.runAutomationBngStatsReport)
				r.Get("/automation/database-backup", s.getAutomationDatabaseBackup)
				r.Patch("/automation/database-backup", s.patchAutomationDatabaseBackup)
				r.Post("/automation/database-backup/run", s.runAutomationDatabaseBackup)
				r.Get("/backup/b2", s.getSettingsB2Backup)
				r.Patch("/backup/b2", s.patchSettingsB2Backup)
				r.Post("/backup/b2/test", s.testSettingsB2Backup)
				r.Get("/automation/history", s.getAutomationExecutionHistory)
				r.Get("/automation", s.getAutomationOverview)
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
				r.Use(s.requirePermissionMiddleware("pops.manage", "*"))
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
				r.Use(s.requirePermissionMiddleware("devices.manage", "devices.collect", "devices.backup", "*"))
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
			r.Get("/bng-vlans", s.listBNGCollectedVLANs)
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
			r.Get("/network/cables/{id}", s.getNetworkCable)
			r.Get("/network/poles", s.listNetworkPoles)
			r.Get("/aggregates", s.commercialAggregates)
			r.Get("/totals-history", s.commercialTotalsHistory)
			r.Get("/comparison", s.commercialMonthComparison)
			r.Get("/reports/export", s.commercialReportsExport)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("commercial.manage", "connections.manage", "pops.manage", "*"))
				r.Post("/localities", s.createLocality)
				r.Patch("/localities/{id}", s.patchLocality)
				r.Delete("/localities/{id}", s.deleteLocality)
				r.Post("/localities/check-vlan-share", s.checkLocalityVLANShare)
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
				r.Post("/network/projects/import/kml", s.importNetworkProject)
				r.Patch("/network/projects/{id}", s.patchNetworkProject)
				r.Delete("/network/projects/{id}", s.deleteNetworkProject)
				r.Post("/network/ctos", s.createNetworkCto)
				r.Post("/network/ctos/bulk", s.bulkNetworkCtos)
				r.Post("/network/ctos/link-olt", s.linkNetworkCtosOlt)
				r.Patch("/network/ctos/{id}", s.patchNetworkCto)
				r.Delete("/network/ctos/{id}", s.deleteNetworkCto)
				r.Post("/network/splice-boxes", s.createNetworkSpliceBox)
				r.Post("/network/splice-boxes/bulk", s.bulkNetworkSpliceBoxes)
				r.Patch("/network/splice-boxes/{id}", s.patchNetworkSpliceBox)
				r.Delete("/network/splice-boxes/{id}", s.deleteNetworkSpliceBox)
				r.Post("/network/cables", s.createNetworkCable)
				r.Post("/network/cables/bulk", s.bulkNetworkCables)
				r.Patch("/network/cables/{id}", s.patchNetworkCable)
				r.Delete("/network/cables/{id}", s.deleteNetworkCable)
				r.Post("/network/poles", s.createNetworkPole)
				r.Post("/network/poles/bulk", s.bulkNetworkPoles)
				r.Patch("/network/poles/{id}", s.patchNetworkPole)
				r.Delete("/network/poles/{id}", s.deleteNetworkPole)
				r.Post("/reports/send-telegram", s.commercialReportsSendTelegram)
			})
		})
		r.Route("/maintenance", func(r chi.Router) {
			r.Get("/windows", s.listMaintenanceWindows)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("alerts.manage", "*"))
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
				r.Use(s.requirePermissionMiddleware("alerts.verify", "alerts.manage", "*"))
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
				r.Use(s.requirePermissionMiddleware("alerts.manage", "*"))
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
				r.Use(s.requirePermissionMiddleware("integrations.manage", "*"))
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
			r.Get("/devices/{id}/run", s.pingRunDevice)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("devices.collect", "*"))
				r.Post("/devices/{id}/run", s.pingRunDevice)
			})
		})

		r.Route("/telemetry", func(r chi.Router) {
			r.Get("/devices/{id}/latest", s.telemetryLatest)
			r.Get("/history", s.telemetryHistory)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("devices.collect", "mikrotik.collect", "switch.collect", "*"))
				r.Post("/devices/{id}/collect", s.telemetryCollect)
			})
		})

		r.Route("/interfaces", func(r chi.Router) {
			r.Get("/devices/{id}", s.listDeviceInterfaces)
			r.Get("/history", s.interfacesHistory)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("devices.collect", "mikrotik.collect", "switch.collect", "*"))
				r.Post("/devices/{id}/refresh", s.refreshDeviceInterfaces)
				r.Post("/devices/{id}/realtime", s.realtimeDeviceInterfaces)
				r.Put("/devices/{id}/metadata", s.putDeviceInterfaceMetadata)
			})
		})

		r.Route("/olt", func(r chi.Router) {
			r.Get("/devices", s.listOLTDevices)
			r.Get("/devices/{id}", s.getOLTDevice)
			r.Get("/devices/{id}/snmp-debug", s.getOLTSnmpDebug)
			r.Get("/reports/history", s.getOLTReportsHistory)
			r.Post("/onu-search", s.searchOLTOnus)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("olt.collect", "olt.onu_manage", "*"))
				r.Post("/devices/{id}/refresh", s.refreshOLTDevice)
				r.Post("/devices/{id}/snmp-debug", s.postOLTSnmpDebug)
				r.Post("/devices/{id}/onu-report", s.reportOLTOnu)
				r.Post("/devices/{id}/onu-serial-search", s.searchOLTOnuBySerial)
				r.Post("/devices/{id}/unauthorized-onus", s.listOLTUnauthorizedOnus)
				r.Post("/devices/{id}/onu-authorize-preview", s.previewAuthorizeOLTOnu)
				r.Post("/devices/{id}/onu-authorize", s.authorizeOLTOnu)
				r.Post("/devices/{id}/onu-deauthorize", s.deauthorizeOLTOnu)
				r.Post("/devices/{id}/discover-vlans", s.discoverOLTVlanCatalog)
			})
		})

		r.Route("/snmp-walk", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("tools.execute", "devices.collect", "*"))
				r.Post("/devices/{id}/run", s.snmpWalkDeviceRun)
				r.Get("/devices/{id}/jobs/{jobId}", s.snmpWalkJobGet)
				r.Get("/devices/{id}/candidates", s.snmpWalkCandidates)
			})
		})

		r.Route("/bng", func(r chi.Router) {
			r.Get("/devices", s.bngListDevices)
			r.Get("/devices/{id}/overview", s.bngDeviceOverview)
			r.Get("/devices/{id}/subscribers/live", s.bngDeviceSubscribersLive)
			r.Get("/devices/{id}/sessions", s.bngDeviceSessions)
			r.Get("/devices/{id}/sessions/report", s.bngDeviceSessionReport)
			r.Get("/devices/{id}/sessions/history", s.bngDeviceLoginEvents)
			r.Get("/devices/{id}/sessions/lookup", s.bngDeviceSessionLookup)
			r.Get("/devices/{id}/sessions/lookup/auth", s.bngDeviceSessionAuthLogs)
			r.Get("/devices/{id}/sessions/traffic-rate", s.bngDeviceSessionTrafficRate)
			r.Post("/devices/{id}/sessions/live-batch", s.bngDeviceSessionsLiveBatch)
			r.Get("/devices/{id}/auth-records", s.bngDeviceAuthRecords)
			r.Get("/devices/{id}/sessions/collect/status", s.bngDeviceSessionsCollectStatus)
			r.Get("/stats/history", s.bngStatsHistory)
			r.Get("/sessions", s.bngSessions)
			r.Get("/sessions/search", s.bngSessionsSearch)
			r.Get("/auth/logs", s.bngAuthLogs)
			r.Get("/traffic/users", s.bngTrafficUsers)
			r.Get("/stats/summary", s.bngStatsSummary)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("bng.collect", "*"))
				r.Post("/devices/{id}/sessions/collect", s.bngDeviceSessionsCollect)
				r.Post("/devices/{id}/collect", s.bngDeviceCollect)
			})
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
				r.Use(s.requirePermissionMiddleware("reports.send", "reports.manage", "*"))
				r.Post("/system/{id}/telegram", s.systemReportTelegram)
			})
		})

		r.Get("/map/equipment-points/{deviceId}", s.mapEquipmentPointDetail)
		r.Get("/map/equipment-points", s.mapEquipmentPoints)
		r.Get("/map/connection-points", s.mapConnectionPoints)
		r.Get("/map/infrastructure-points", s.mapInfrastructurePoints)
		r.Get("/map/search", s.mapSearch)
		r.Get("/map/locate", s.mapLocate)
		r.Get("/map/locality-center", s.mapLocalityCenter)
		r.Get("/map/project-center", s.mapProjectCenter)
		r.Get("/map/nearest-ctos", s.mapNearestCtos)

		r.Get("/overview/summary", s.overviewSummary)
		r.Get("/overview/top-latency", s.overviewTopLatency)
		r.Get("/dashboard/analytics", s.dashboardAnalytics)
		r.Get("/dashboard/data-gaps", s.dashboardDataGaps)
		r.Get("/dashboard/olt-capacity", s.dashboardOltCapacity)

		r.Route("/fleet", func(r chi.Router) {
			r.Get("/dashboard", s.fleetDashboard)
			r.Get("/settings", s.getFleetSettings)
			r.Get("/me/driver", s.fleetMeDriver)
			r.Get("/users", s.listFleetUsersLite)
			r.Get("/cost-centers", s.listFleetCostCenters)
			r.Get("/fuels", s.listFleetFuels)
			r.Get("/stations", s.listFleetStations)
			r.Get("/stations/{id}", s.getFleetStation)
			r.Get("/vehicles", s.listFleetVehicles)
			r.Get("/vehicles/{id}", s.getFleetVehicle)
			r.Get("/vehicles/{id}/autofill", s.fleetVehicleAutofill)
			r.Get("/vehicles/{id}/summary", s.fleetVehicleSummary)
			r.Get("/drivers", s.listFleetDrivers)
			r.Get("/driver-vehicles", s.listFleetDriverVehicles)
			r.Get("/fuelings", s.listFleetFuelings)
			r.Get("/expenses", s.listFleetExpenses)
			r.Get("/expenses/export/csv", s.exportFleetExpensesCSV)
			r.Get("/expense-types", s.listFleetExpenseTypes)
			r.Get("/alerts", s.listFleetAlerts)
			r.Get("/reports/{kind}", s.fleetReportCSV)
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("fleet.manage", "reports.send", "*"))
				r.Post("/reports/{kind}/telegram", s.fleetReportTelegram)
			})
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermissionMiddleware("fleet.manage", "*"))
				r.Patch("/settings", s.patchFleetSettings)
				r.Post("/cost-centers", s.createFleetCostCenter)
				r.Patch("/cost-centers/{id}", s.patchFleetCostCenter)
				r.Post("/fuels", s.createFleetFuel)
				r.Patch("/fuels/{id}", s.patchFleetFuel)
				r.Post("/stations", s.createFleetStation)
				r.Patch("/stations/{id}", s.patchFleetStation)
				r.Post("/vehicles", s.createFleetVehicle)
				r.Post("/vehicles/import/csv", s.importFleetVehiclesCSV)
				r.Patch("/vehicles/{id}", s.patchFleetVehicle)
				r.Post("/drivers", s.createFleetDriver)
				r.Post("/drivers/import/csv", s.importFleetDriversCSV)
				r.Patch("/drivers/{id}", s.patchFleetDriver)
				r.Post("/driver-vehicles", s.createFleetDriverVehicle)
				r.Delete("/driver-vehicles/{id}", s.deleteFleetDriverVehicle)
				r.Post("/fuelings", s.createFleetFueling)
				r.Post("/fuelings/quick", s.createFleetFueling)
				r.Post("/expenses", s.createFleetExpense)
				r.Post("/expenses/import/csv", s.importFleetExpensesCSV)
				r.Post("/expenses/purge", s.purgeFleetExpenses)
				r.Post("/expense-types", s.createFleetExpenseType)
				r.Patch("/expense-types/{id}", s.patchFleetExpenseType)
				r.Post("/alerts/{id}/ack", s.ackFleetAlert)
			})
		})
	})

	if cfg.EmbeddedUI {
		ui := embedui.Handler(log)
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			if strings.HasPrefix(req.URL.Path, "/api/") {
				writeErr(w, http.StatusNotFound, "NOT_FOUND", "rota não encontrada", nil)
				return
			}
			ui.ServeHTTP(w, req)
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
			if strings.HasPrefix(req.URL.Path, "/api/") {
				writeErr(w, http.StatusMethodNotAllowed, "METHOD", "método não permitido", nil)
				return
			}
			ui.ServeHTTP(w, req)
		})
	}

	if s.DB() != nil {
		s.ensureBackgroundSchedulers()
	}

	return chain(cfg, log, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "netquasar-backend"})
}
