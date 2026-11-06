workspace "university-ERP replacement" "As-is (legacy university ERP) and target architecture for an open-source, multi-tenant replacement." {

    model {

        // ── People ────────────────────────────────────────────────────────────
        sachb_personal = person "Personalsachbearbeiter:in" "HR administration staff"
        sachb_finance  = person "Finanzsachbearbeiter:in" "Finance administration staff"
        kassenwart     = person "Kassenverwalter:in" "Cashier"
        controlling    = person "Controlling / Hochschulleitung" "Management reporting"
        revision       = person "Revision / Prüfung" "Audit / internal revision"
        it_ops         = person "IT-Betrieb" "Operations (Hochschul-RZ)"
        contributor    = person "Open-source contributor" "Develops and maintains the replacement"

        // ── External systems (shared by as-is and target) ─────────────────────
        lbv       = softwareSystem "LBV / Bezügestelle" "State payroll office" "External"
        bank      = softwareSystem "Banken / EBICS" "SEPA payment traffic and statements" "External"
        elster    = softwareSystem "Finanzamt / ELSTER" "Tax filing via ERiC" "External"
        statistik = softwareSystem "Statistik / Landesamt" "Statistics offices (per state)" "External"
        idp       = softwareSystem "Identity Provider" "Shibboleth / OIDC (DFN-AAI)" "External"
        dms       = softwareSystem "DMS" "Document management system" "External"
        the reference source  = softwareSystem "the reference implementation Campus" "Campus management (APP/STU/EXA/DOC)" "External"
        qis       = softwareSystem "QIS" "External campus/exam system" "External"
        saphcm    = softwareSystem "SAP HCM" "External HR system" "External"

        // ══ AS-IS ═════════════════════════════════════════════════════════════
        legacy = softwareSystem "legacy university ERP (legacy)" "legacy university ERP: the HR module, the finance module/the finance module, the cost-accounting module, the cash-book module. Replaced by the target system." {

            legacy_web = container "Web-Oberfläche / Portal" "Role-specific web UI" "Java/Web"
            legacy_sva = container "the HR module application" "Personal- und Stellenverwaltung" "Java" {
                fw_vorgang = component "Vorgangssteuerung" "sv_vorgang*"
                fw_workflow = component "Workflow" "wof"
                fw_rechte = component "Rechte & Rollen" "sv_rechte_sachb"
                fw_katalog = component "Katalogverwaltung" "k_*"
                fw_meta = component "Metadata / UI" "db_tabellen, db_spalten, db_graph_*"
            }
            legacy_mbs = container "the finance module / the finance module application" "Finanz- und Sachmittelverfahren" "Java"
            legacy_cob = container "the cost-accounting module application" "Kosten- und Leistungsrechnung" "Java"
            legacy_kbs = container "the cash-book module application" "Kassenbuchungssystem" "Java"
            legacy_db  = container "PostgreSQL" "One DB, four module schemas, no foreign keys" "PostgreSQL 17" "Database"
            legacy_batch = container "Batch / Schnittstellen" "File/DB interfaces" "Batch"
            legacy_docs = container "Dokumentenablage" "DB / filesystem / DMS" "Storage"
        }

        // ══ TARGET ════════════════════════════════════════════════════════════
        target = softwareSystem "University ERP Replacement" "New open-source, multi-tenant platform (Go + Angular, k8s)." {

            target_web = container "Web UI" "Angular SPA (standalone + signals, Material/CDK)" "TypeScript"
            target_api = container "API" "Modular monolith: REST/JSON + OpenAPI" "Go" {
                core_authz    = component "AuthZ / Roles" "Claims -> roles, four-eyes"
                core_audit    = component "Audit" "Append-only, hash-chained audit stream"
                core_workflow = component "Workflow" "Approvals, cases"
                core_catalog  = component "Catalogues" "Reference data"
                core_report   = component "Reporting" "Print/centre, PDF templates"
                core_reg      = component "Regulatory adapters" "Per-Bundesland rules"
                mod_personal  = component "Personal module" "Positions, pay, travel, time"
                mod_finance   = component "Finance module" "Ledger, budget, invoices, procurement"
                mod_kasse     = component "Kasse module" "Cash book, payments, dunning, foundations"
                mod_klr       = component "KLR module" "Budget structure, apportionment"
                core_etl      = component "ETL / anti-corruption" "Legacy schema -> new schema"
            }
            target_worker = container "Worker" "Async jobs: imports, exports, ETL, reports" "Go"
            target_connectors = container "Connectors" "ERiC/ELSTER, EBICS (native deps)" "Go + cgo"
            target_pdf = container "PDF service" "HTML -> PDF (Bescheide, certificates)" "Gotenberg"
            target_scheduler = container "Scheduler" "Nightly batch, dunning, statement fetch" "k8s CronJob"
            target_db = container "PostgreSQL (per tenant)" "CloudNativePG, database per tenant, read replicas" "PostgreSQL" "Database"
            target_pooler = container "PgBouncer" "Transaction pooling, one pool per tenant" "PgBouncer"
            target_objects = container "Object storage" "Documents, exports, ETL staging, object-lock archive" "S3/MinIO" "Database"
            target_redis = container "Redis" "Sessions, rate limiting, cache" "Redis" "Database"
            target_ingress = container "Ingress" "TLS, per-tenant hostnames" "nginx/Traefik"
            target_sftp = container "File gateway" "SFTP for LBV/statistics files" "SFTP"
            target_obs = container "Observability" "OTel Collector, Prometheus, Grafana, Loki" "Observability"
        }

        // ── Relationships: as-is ──────────────────────────────────────────────
        sachb_personal -> legacy_web "verwaltet Personal, Stellen, Reisen"
        sachb_finance  -> legacy_web "verwaltet Haushalt, Rechnungen, Beschaffung"
        kassenwart     -> legacy_web "führt Kassenbuch, Zahlungsverkehr"
        controlling    -> legacy_web "Controlling, Auswertungen"
        revision       -> legacy_web "Revision, Protokolle"
        it_ops         -> legacy "betreibt"

        legacy_web -> legacy_sva "uses"
        legacy_web -> legacy_mbs "uses"
        legacy_web -> legacy_cob "uses"
        legacy_web -> legacy_kbs "uses"
        legacy_sva -> legacy_db "reads/writes" "schema sva4"
        legacy_mbs -> legacy_db "reads/writes" "schema mbs"
        legacy_cob -> legacy_db "reads/writes" "schema cob"
        legacy_kbs -> legacy_db "reads/writes" "schema kbs"
        legacy_batch -> legacy_db "file/DB interfaces"
        legacy_sva -> legacy_docs "documents"
        legacy_mbs -> legacy_docs "documents"
        legacy_sva -> lbv "Bezüge-/Besoldungsmeldungen (file)"
        legacy_batch -> bank "SEPA / MT940"
        legacy_batch -> elster "Umsatzsteuer (ERiC)"
        legacy_batch -> statistik "Personal-/Finanzstatistik (file)"
        legacy_web -> idp "authentication (Shibboleth)"
        legacy -> the reference source "Stammdaten / Kostenstellen"
        legacy -> qis "Personendaten-Transfer"
        legacy -> saphcm "HR master data"
        legacy -> dms "documents"

        // ── Relationships: target ─────────────────────────────────────────────
        sachb_personal -> target_web "verwaltet Personal, Stellen, Reisen"
        sachb_finance  -> target_web "verwaltet Haushalt, Rechnungen, Beschaffung"
        kassenwart     -> target_web "führt Kassenbuch, Zahlungsverkehr"
        controlling    -> target_web "Controlling, Auswertungen"
        revision       -> target_web "Revision, Protokolle"
        it_ops         -> target "betreibt"
        contributor    -> target "entwickelt"

        target_web -> target_api "REST/JSON" "HTTPS"
        target_api -> target_pooler "SQL"
        target_pooler -> target_db "pooled connections"
        target_api -> target_redis "sessions/cache"
        target_api -> target_objects "documents"
        target_api -> target_worker "enqueues jobs" "River"
        target_worker -> target_db "reads/writes"
        target_worker -> target_objects "exports, staging"
        target_worker -> target_connectors "native-dep jobs"
        target_worker -> target_pdf "render documents"
        target_scheduler -> target_worker "triggers batch"
        target_connectors -> target_db "job state"
        target_ingress -> target_web "routes" "per-tenant hostnames"
        target_web -> idp "OIDC/SAML (PKCE)"
        target_api -> idp "token validation"
        target_sftp -> target_worker "file interfaces"
        target_connectors -> lbv "Bezüge (file/SFTP)"
        target_connectors -> bank "SEPA / EBICS / MT940"
        target_connectors -> elster "Umsatzsteuer (ERiC)"
        target_worker -> statistik "Statistik (file)"
        target_api -> target_obs "telemetry"
        target_worker -> target_obs "telemetry"
        target_api -> the reference source "data exchange (optional connector)"
        target_api -> qis "data exchange (optional connector)"
        target_api -> saphcm "data exchange (optional connector)"
        target_worker -> dms "optional DMS connector"

        // ── Strangler: legacy -> target ───────────────────────────────────────
        legacy_db -> target_worker "ETL: legacy schema -> new schema (parallel run)"

    }

    views {

        systemContext legacy "AsIs-Context" "As-is system context (legacy legacy university ERP)." {
            include *
            autoLayout
        }

        container legacy "AsIs-Containers" "As-is containers." {
            include *
            autoLayout
        }

        component legacy_sva "AsIs-SvaComponents" "Legacy framework components (replicated per module schema)." {
            include *
            autoLayout
        }

        systemContext target "Target-Context" "Target system context." {
            include *
            autoLayout
        }

        container target "Target-Containers" "Target containers." {
            include *
            autoLayout
        }

        component target_api "Target-ApiComponents" "Target API components (modular monolith)." {
            include *
            autoLayout
        }

        styles {
            element "Person" {
                shape Person
                background "#08427b"
                color "#ffffff"
            }
            element "External" {
                background "#999999"
                color "#ffffff"
            }
            element "Database" {
                shape Cylinder
            }
            element "Container" {
                background "#438dd5"
                color "#ffffff"
            }
            element "Component" {
                background "#85bbf0"
                color "#000000"
            }
        }

    }

}
