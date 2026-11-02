# Schema analysis — reverse-engineered data model

This is the reverse-engineered model. Because the database has **no declared
foreign keys** (§ "No FKs"), everything below is derived from table/column
naming, shared columns, PK structure and the dump's own metadata. Each section
states what is *measured* versus *inferred*.

## 0. Structural rules observed

1. **Every module schema repeats the framework.** `sv_*`, `wof`, `inst`,
   `db_*`, `k_*`, `r_*` appear in each of `sva4` / `mbs` / `cob` (and `kbs` in
   reduced form). There is no shared central schema.
2. **Keys are surrogate integers** (`*_id` with a sequence) — 1,558 PKs over
   1,536 tables. Some tables have no PK.
3. **Relationships are by convention**, not constraint: `X_to_Y` link tables and
   `<entity>_id` columns. `*_to_*` tables are the many-to-many links
   (e.g. `ansc_to_inst`, `bga_to_mbs`, `proj_to_inst`, `gvp_to_pbv`).
4. **Audit via triggers**, not FKs (`<table>db`/`da`/`ib`/`ub`, `db_einfuegemarke`).
5. **Catalogues are `k_*`** and are the richest, widest part of the schema.

---

## 1. `sva4` — Personal- und Stellenverwaltung (module `the HR module-GX`)

636 tables, 224 with data. Domain clusters (framework excluded):

| Cluster | Tables | Domain (inferred) |
|---|---|---|
| `pb*` | `pbv` `pbw` `pbz` `pbe` `pbh` `pbk` `pbl` `pbu` `pbv_gehalt` `pbv_vorzeiten` `pbw_bewertung` `pbw_status` `pbz_betraege` | Core personnel/remuneration case processing (`PB*` = Personal-/Bezüge-*) |
| `pg*` | `pgd` `pgd_dgti` `pgd_to_bgr` `pgd_to_sas` `pgd_ze_chip` `pgp` | Personnel master/assignment data |
| `p*` (misc) | `pko` `pkr` `pfa` `pfi` `plv` `pmi` `pnt` `pts` `pus` `pvo` `pvr` `pvu` `pzd` `pze` `pzk` `pzu` `pzv` `pat` `paw` `paz` `pab` `pad` `pak` `pas` `pdo` `pdp` `pdv` | Personnel sub-processes (many small process tables) |
| Travel / relocation | `rka_*` (Reisekostenabrechnung: anlagen, ausland, beförderung, fahrkosten, übernachtung, wegstrecke …), `ter_*` (Trennungsgeld/Umzug), `gvp_*` (geldwerte Vorteile), `ggaa` | Travel-expense & relocation processing |
| Payroll interfaces | `bw_lbv_*` (`_base`, `_beleg`, `_his0001`…`_his0099`, `_hisduz`, `_hiseau*`, `_steuermeldung`), `nrw_*` (state-specific), `lbv_*`, `h1_sva_transfer` | Interfaces to payroll / LBV (state payroll office) and SAP HCM (`H1`) |
| Working time | `az_config` `az_config_details` `az_model` `asz` `trg` `trg_monat` `trg_tage` `ura` `sdp` | Working-time & leave models |
| Social insurance | `sas` `sas_stand` `sas_to_medienkanal` `sas_kommentar` `sbu` `sko` `shv` | Social-insurance / health-fund reporting |
| Structural | `bgr` `bgr_to_sas` `sem` `sem_kosten` `sgd` `sgd_prot` `sko_to_pbe` `kpt_*` `aproj` `aprge` `bta*` | Org units, cost assignment, projects |
| External/3rd-party | `qis_*`, `meta_person`, `meta_rolle`, `person_taa_config` | QIS (external campus system) integration, person metadata |
| Payments | `import_payment` `import_payment_det` `calc_transaction` `entry_transaction` | Payment import & transaction staging |
| Storno | `vorgangsstornierungen` | Reversal of cases |

Person master table is likely `meta_person` / `pgd` (inferred, not confirmed).

## 2. `mbs` — Finanz- und Sachmittelverfahren (modules `the finance module-UNIX` / `the finance module-GX`)

556 tables, 199 with data. The largest, most heterogeneous module — it is the
finance/resource core. Clusters:

| Cluster | Tables | Domain (inferred) |
|---|---|---|
| Financial accounting (`fibu_*`) | `fibu_konten` `fibu_journal` `fibu_debitoren` `fibu_kreditoren` `fibu_abschluss` `fibu_auswertung*` `fibu_gwg_afa` `fibu_gwg_konten` `fibu_klassen` `fibu_kontenstand` `fibu_report` `fibu_sonder` `fibu_zuordnung` `fibu_vorbelegung` | General ledger, debtors/creditors, asset depreciation (GWG/AfA), reporting |
| Budget / Haushalt | `konsens_mv_*` (Haushaltskonsens), `ein_zeitansatz` `ein_detail` `ein_zuschlag` `zst_*` (Zahlstellen: `zst_jahrabschl`, `zst_monatsabr`, `zst_kontostand`, `zst_tagabschl`), `tit` `titdr` `kontogen` `kontenalle` | Budgeting, budget-consensus workflow, cost centres (Zahlstellen) |
| Incoming invoices | `rechnungseingangsbuch` `rechnungseingangsbuchorg` `beleg` `belegzeit` `rmkasse` `rmzahlung` `rmbelegposition` `rmposteninfo` `rmabrechobj` | Invoice register (REB), documents, cash/clearing (`rm*`) |
| Procurement / tendering | `vergabe` `vergabe_uvgo` `vergabe_vgv` `vergabe_vob` `anfr` `anfp` `ange` `angp` `auft` `aufp` `lieferant` `warenannahme` | Tendering (UVgO/VgV/VOB), requests, offers, orders, suppliers, goods receipt |
| Real estate / property (`glm_*`) | `glm_ek` `glm_se` `glm_sw` `glm_lrahm` `glm_swbundle` `glm_swliz` `glm_ekattr*` `glm_swliz*` `geb` `gebber` `flur` `gemark` `boden` `miete` `pflege` `lage` `ort` | Property & lease management (Grundstücke/Liegenschaften/Mietverträge), buildings, parcels |
| Inventory / assets (`iv*`) | `ivab` `ivaen` `ivaf` `ivasp` `ivleih` `ivprot` `ivru` `ivsbga` `ivsplit` `ivst` `ivumb` `ivsexpcob` | Inventory, lending, transfers, export to COB |
| Library | `ein_bibliothek` `ein_biblfakt` | Library acquisition/factoring |
| Personnel cost | `pers` `ein_personal` `lohn_to_koa` | Personnel and payroll-cost assignment |
| Students | `stud` `studg` `ein_student` `ein_studfakt` `stu_booking_response` | Student-related finance |
| Banking / SEPA | `bic_sepa_credit_transfer` `bic_sepa_direct_debit` `direct_debit_mandate` `creditor_id` `bank_scl` `kto_auszug` | SEPA payments, mandates, bank statements |
| Projects / funding | `prgi` `prgn` `prgs` `proaus` `rpplan` `rpvor` `rna` `rnaber` `rnakfa` | Third-party funded projects and plans |
| Master/templates | `vstamm` `pstamm` `astamm` `stamm` | Base templates per object type |
| Misc | `nutzer` `nutzer_fsv` `mahn` `tel` `schaden` `versorg` `veranst` | Users, dunning, damage, pension, events |

## 3. `cob` — Kosten- und Leistungsrechnung (module `the cost-accounting module-GX`)

244 tables, 99 with data. Cost & activity accounting.

| Cluster | Tables | Domain (inferred) |
|---|---|---|
| Budget structure | `busa` `arch_busa` `imp_busa` `tmp_imp_busa` `alt_fikr` `alt_hier` `alt_inst` `alt_proj` | Budget structure (`BUSA`) + archived/imported copies + alternative hierarchies |
| Reference quantities | `bzgr` `bzgr_empf` `p_bzgrko` | Bezugsgrößen (reference/charge quantities) and recipients |
| Cost/activity types & reports | `l100` `l110` `l200` `l210` `l220` `l910` `lcr100`…`lcr433` `lcr1x7` `lcr204_b` `lcr204_h` `lcr210_exp` `lcr326_exp` | Activity/cost lines and the `LCR*` report tables |
| Apportionment / charging (`kpum`, `um*`) | `kpum` `kpum_auto` `kpum_grp` `kpum_param` `kpumauto` `umks` `umks_e` `umvm` `umvm_aus` `umvm_vert` `vmpr` | Cost apportionment (`KPUM`/`UM*` = Umlage/Verrechnung) |
| Projects / students | `proj` `proj_to_inst` `proj_to_pbv` `proj_to_stug` `stud` `stug` `stupl` `ff_to_stug` `su_imp_stud_view` | Projects and student/study-programme linkage |
| Third-party funds | `drittmittelherk` `dta_verzvtbu` `rna_to_nakfa` | Origin of third-party funding |
| Ansatz | `ansc` `ansc_to_inst` | Budget target (Ansatz) |
| Evaluation & tooling | `eval` `eval_kpum_zuord` `reporttypen` `tabkey` `tabkeyecht` `trees` `tree_cfg` `felder_to_p_select` | Evaluations, report types, UI trees |

## 4. `kbs` — Kassenbuchungssystem (module `the cash-book module`)

94 tables, 18 with data. Cash book / cashier system.

| Cluster | Tables | Domain (inferred) |
|---|---|---|
| Cash book | `kass` `schalterbuch` `sb` `sbprot` `bu` `buha` `geing` `dr` `drprot` | Cash book (`Schalterbuch`), collective bookings, bookings, receipts |
| Orders | `ao` `aoar` `feao` | Ausgabeanordnung / payment orders |
| Payment traffic | `zawe` `zawe_zusatz` `zabe` `zbu` `zp_bankdaten` `zp_stammdaten` `zv_freigabe` `transfrei` `mt940` `mt940ar` `datmt940` `kreditkarten` `mandat` | Payment routes (Zahlweg), bank master, releases, MT940 statements, cards, mandates |
| Cheques | `scheck` `bln_verst_einz` | Cheques / individual collections |
| Dunning | `mahn` `mahnprot` | Dunning |
| Foundations | `stift` `stiftprot` `tgas` `titkto` | Foundation cash books, accounts |
| Tax office | `finanzamt` `finanzamtprot` `mwst_art` | Tax-office data, VAT types |
| Bank reconciliation | `rueckausz_arch` `rueckauszdatum` `rueckfibu_arch` `rueckmeldungfibu` `datrueckausz` `datrueckfibu` | Statement reconciliation, return postings to FiBu |
| Cases | `vorg` `vorgprot` | Cashier cases |
| Person account | `pers` | Person account |
| Exports | `sos_export` `stu_export` `kbsans` | Exports (SOS/STU/ANS) |

---

## 5. Cross-module linkage (inferred)

The `*_to_*` link tables show the intended seams between modules:

| Link table | Connects |
|---|---|
| `bga_to_mbs` (cob) | cost accounting ↔ Sachmittel (`mbs`) |
| `proj_to_pbv`, `proj_to_inst`, `proj_to_stug` (cob) | projects ↔ personnel cases / institutions / study programmes |
| `ivsexpcob` (mbs) | inventory export to COB |
| `ansc_to_inst` (cob) | budget target ↔ institution |
| `sgd`/`sem`/`kpt_*` (sva4) | personnel ↔ cost centres |
| `kbsans`, `stu_export` (kbs) | cash ↔ external systems |

These are the only places the schema hints at cross-module relationships;
otherwise integrity is application-enforced.

## 6. What is NOT determinable from the dump

- Actual business rules / validation logic (lives in 3,601 DB functions and the
  application).
- The authoritative join paths (no FKs).
- Which `__`-prefixed vs plain table is live for a given module.
- Column semantics for the ~38k columns — only 6 column comments exist in the
  whole dump.
