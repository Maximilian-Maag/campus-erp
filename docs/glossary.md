# Glossary — German domain terms

Decoded from table/column naming. Confidence is marked: **[c]** confirmed in
source text, **[i]** inferred from convention, **[?]** uncertain.

## Organisational / generic

| Term | Meaning |
|---|---|
| Einrichtung / `inst` | institution / organisational unit **[c]** |
| Hochschule | higher-education institution **[c]** |
| Sachgebiet (`sachb`) | organisational area / desk **[i]** |
| Vorgang | business transaction / case **[c]** (framework `sv_vorgang*`) |
| Teilvorgang | sub-transaction **[i]** |
| Sachbearbeiter | case worker **[i]** |
| Wof / Workflow | workflow **[c]** |
| Stelle / Stellen | position(s) **[c]** |
| Person | person **[c]** |
| Mandant | tenant / client **[i]** |

## Finance (`mbs`, `cob`, `kbs`)

| Term | Meaning |
|---|---|
| Haushalt / Titel | budget / budget line **[c]** |
| Ansatz | budget target/appropriation **[i]** (`ansc`) |
| Budgetstruktur / `busa` | budget structure **[i]** |
| Bezugsgröße / `bzgr` | reference/charge quantity **[i]** |
| Zahlstelle / `zst` | pay office / cost centre **[i]** |
| Konten / Konto | accounts / account **[c]** |
| Fibu | Finanzbuchhaltung — general ledger **[c]** |
| Debitoren / Kreditoren | debtors / creditors **[c]** |
| Beleg | voucher / document **[c]** |
| Rechnungseingangsbuch | incoming-invoice register **[c]** |
| Vorbelegung | pre-posting / pre-assignment **[i]** |
| Abschluss | closing (period/year-end) **[c]** |
| Auswertung | evaluation / report **[c]** |
| GWG / AfA | low-value assets / depreciation **[i]** |
| Mahnung / `mahn` | dunning **[c]** |
| Rückauszug | bank statement return / reconciliation **[i]** |
| Zahlweg / `zawe` | payment route **[i]** |
| Freigabe | release / approval **[c]** |
| Umsatzsteuer / `mwst_art` | VAT / VAT type **[c]** |
| Drittmittel | third-party (external) funding **[c]** |
| Umlage / `um*`, `kpum` | apportionment / cost allocation **[i]** |
| Kosten- und Leistungsrechnung (KLR) | cost & activity accounting **[c]** |
| Vergabe (UVgO/VgV/VOB) | public tendering under the named procurement rules **[c]** |
| Anfrage / Angebot / Auftrag | request / offer / order **[c]** |

## Personnel (`sva4`)

| Term | Meaning |
|---|---|
| Besoldung | (civil-servant) pay **[c]** |
| Bezüge | remuneration **[c]** |
| Tarif | pay scale / collective agreement **[c]** |
| Dienstart | type of service/employment **[i]** |
| Amtsdienstbezüge | official service pay **[i]** |
| LBV | Landesamt für Besoldung und Versorgung — state pay office **[i]** |
| Reisekosten / `rka` | travel expenses **[i]** |
| Trennungsgeld / `ter` | separation allowance (relocation) **[i]** |
| Geldwerte Vorteile / `gvp` | benefits in kind **[i]** |
| Arbeitszeit / `az` | working time **[i]** |
| Urlaub / `ura` | leave **[i]** |
| Sozialversicherung / `sas` | social insurance **[i]** |
| Krankenkasse | health-insurance fund **[c]** |
| Storno | reversal / cancellation **[c]** |
| Vorzeiten | prior service periods **[i]** |
| Zulage | bonus/allowance **[i]** |
| Beschäftigte | employees **[c]** |
| Bewerber | applicant **[c]** |

## Cash (`kbs`)

| Term | Meaning |
|---|---|
| Kasse | cash desk **[c]** |
| Schalterbuch | counter book **[i]** |
| Ausgabeanordnung / `ao` | payment order **[i]** |
| Scheck | cheque **[c]** |
| Stiftung | foundation **[c]** |
| Finanzamt | tax office **[c]** |
| Kassenbuchungssystem | cash-book system **[c]** |

## Campus (the reference implementation — not in this dump)

| Term | Meaning |
|---|---|
| Bewerbung | application **[c]** |
| Zulassung | admission **[c]** |
| Immatrikulation / Exmatrikulation | matriculation / deregistration **[c]** |
| Rückmeldung | re-registration **[c]** |
| Beurlaubung | leave of absence **[c]** |
| Prüfung | examination **[c]** |
| Veranstaltung | course/event **[c]** |
| Studiengang | study programme **[c]** |
| Doktorand / Promotion | doctoral candidate / doctorate **[c]** |
| Gasthörer | guest student **[c]** |
| Modulhandbuch | module handbook **[c]** |

## Generic column names

| Column | Meaning |
|---|---|
| `*_id` | surrogate key (sequence) |
| `db_einfuegemarke` | insert/audit marker **[i]** |
| `dat_von` / `dat_bis` | valid-from / valid-to **[i]** |
| `name`, `bezeichnung`, `kennzeichen` | name / label / code |
