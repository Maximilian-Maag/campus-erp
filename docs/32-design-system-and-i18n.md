# 32 — The design system and the two languages

The rebuild's interface was twenty-five hand-written screens that each invented their own
markup and wrote their own German copy. That produced a product that looked like five
products and called the same thing by three names. This document is the rule that stops it
from happening again: **a screen carries structure and meaning; the design system carries the
look; the dictionaries carry the words.**

## The look: four layers of tokens

The look is a design system, not a stylesheet that grew: every colour, size, radius, shadow
and duration is named once, and a screen consumes a name. The system lives in
`frontend/src/styles/` and is reached through one entry point, `frontend/src/styles.scss`:

| file | what it is |
| --- | --- |
| `_tokens.scss` | the Material 3 theme (`mat.theme`) plus the `--erp-*` tokens: layout, a 4px spacing scale, radii, three elevations, two durations, and the surfaces/lines/text/feedback a product needs by name |
| `_base.scss` | the document: reset, type, one focus ring, thin scrollbars, `prefers-reduced-motion` |
| `_layout.scss` | the page: the content frame, the `.page-head` of a screen, the card surfaces |
| `_elements.scss` | the vocabulary's skins: buttons, fields, forms, tables, record lists, messages |

`mat.theme` emits the `--mat-sys-*` roles (palette, type scale, state layers) and
`color-scheme: light dark` makes them resolve to the light or the dark set the operating
system asks for. The `--erp-*` layer names what an ERP *means* by them — `--erp-page`,
`--erp-surface`, `--erp-hairline`, `--erp-accent`, `--erp-shadow-1..3`, `--erp-space-*` —
so a component never picks a colour or a size, and re-theming is an edit to `_tokens.scss`.

Two of those names exist because light and dark separate surfaces differently: a card is
`--erp-card` and a field is `--erp-field`. In light mode both are the plain surface and the
hairline border carries the separation; in dark mode the page is nearly as dark as a card,
so the card steps up one tint and the field steps up again.

Two rules make the system hold:

1. **The element is styled, not the screen.** A `<section>` is the page's block stack (a grid
   with one gap, so spacing cannot double or collapse); a `<table>` is a card with a caption
   bar, a sticky blurred header and tabular numerals; a `<button>` is the filled accent action,
   with `.secondary` and `.danger` variants; `input`/`select`/`textarea` are one field;
   `.form` is the grid of fields, `.filter` the slim toolbar that narrows the list under it;
   `.error`, `.notice` and `[role='status']` are the three ways the product tells the user
   something. Because the screens were already written
   as `<section>` + `<table>` + `.form`, they inherit the design without being rewritten —
   and a new screen cannot look different.
2. **A component styles its own *layout* at most.** `app-page` and `app-shell` position
   things; they take every colour, font size, radius and shadow from a token. There is no
   per-screen stylesheet anywhere in `src/app/`.

### A select whose options come from `@for` carries its selection on the options

A `<select [value]="…">` whose `<option>`s are created by `@for` is bound before those options
exist, so the assignment is dropped and the browser's reset selects the first option. The screen
then *displays* one value while the signal — and the request body — carry another: the clerk reads
"alle" while the list asked for the active ones, or "je Mandat" while the run is filed as `RCUR`.

The selection therefore belongs on the option: `[selected]="'' + expr === signal()"`, with the
`'' +` because the option's value is a string once the browser has it while the signal may hold a
number. A spec that only checks the option list cannot see this — it has to read `.value` on the
select after the signal is set to something other than the first option.

### A failed read says what failed

A read that fails carries its own message: the screen names the thing that could not be loaded
and the reason the API gave, in `.error`, rather than a generic "could not load". A `404` is the
one exception — on a screen that asks for a key that may not exist yet, it means "no such
record", not a failure, and the screen shows the empty state. A screen that loads several
things shows the failure of the part that failed and keeps the rest of itself; one broken
sub-read must not blank the page.

### A figure belongs to the selection it was asked for

A screen that shows a figure under a heading — a report, a balance, a list — sets the label
and the figure together. When the selection changes, the figure is cleared with the label and
the request that produced it is dropped if it lands after the change; a response is only
rendered when the request that asked for it is still the one the screen is waiting on. A
figure shown under a new heading belongs to the new selection, never to the old one.

### A picker seeded from its own list waits for the list

A form whose picker is seeded from the options it loads seeds when the options arrive, not when
the form is created — and only if the user has not chosen in the meantime. A key form also
refuses a zero id: the empty option is a prompt, not a value.

### Accessibility is part of the theme, not a pass at the end

Focus is one ring (`--erp-ring`, 2px outline plus a soft halo) on every control; the skip link
is the accent fill; motion is feedback and disappears under `prefers-reduced-motion`; the
palette comes from Material 3's contrast-checked roles, so light and dark are both
AA. `e2e/*.spec.ts` runs axe on the shell and every screen and fails on any violation
(BITV 2.0 / WCAG, ADR-0003, CR-7).

## Shared components

- `app-shell` — the frame: a translucent application bar (product name, language picker), a
  grouped navigation panel (permanent on a wide screen, an overlay that closes on a narrow one),
  and the content area.
- `app-page` — the head of a screen: `<app-page titleKey="page.…" subtitleKey="page.…">`
  renders the `<h1>`, the one-sentence description, and a slot for the screen's action.
- `app-language-picker` — the picker.

The sign-in screen is the one screen that is not an `app-page`: it holds a single decision rather
than a stack of records, so `login.component.ts` renders `.auth` / `.auth-card` — the centred card
in `src/styles/_layout.scss` — with its own `<h1>` and the same one-action rule. The shell shows
no navigation beside it, because the navigation belongs to somebody who has signed in; a
deployment without an issuer has no such wall, so there it is always shown.

## The words: one vocabulary

`docs/glossary.md` is the source of the domain vocabulary — the legacy system's German terms
and what they mean. The interface uses those terms, and only those. Concretely:

- A concept has **one key** and therefore one name on every screen. `common.amount` is
  "Betrag" here, there and everywhere; a screen that wants to say "Summe" is a defect.
- **Labels are nouns in sentence case** ("Zahlungsanordnungen", "Betrag in Cent").
- **Actions are verb phrases in the infinitive that name exactly one action** ("Anlegen",
  "Freigeben", "Tag eröffnen") — never a sentence, never "Hier klicken".
- **A status is the state the backend reports**, translated, not paraphrased.
- English is **British** and uses the same vocabulary: payment order (Zahlungsanordnung),
  payment run (Zahlungslauf), dunning / dunning run (Mahnwesen / Mahnlauf), cost centre
  (Kostenstelle), cost type (Kostenart), cost unit (Kostenträger), budget position
  (Haushaltsstelle), funding source (Mittelquelle), goods receipt (Wareneingang).

## The mechanics: Transloco, typed dictionaries

The product is published in German (`de`, the default) and English (`en`). Transloco does the
switching; the dictionaries are TypeScript, not JSON, so they are type-checked and shipped in
the bundle (a screen can never flash a key while a JSON request is in flight).

```
frontend/src/app/i18n/
  shared.de.ts / shared.en.ts      app frame, navigation, page headings, common terms, statuses
  pages/<area>.de.ts  / .en.ts     one fragment per area of the product (kasse, klr, …)
  messages.de.ts / messages.en.ts  the composers: shared + fragments
  dictionary.ts                    the loader, the supported languages, isLanguage()
  language.service.ts              the active language; persistence; <html lang>
  provide-i18n.ts                  provideI18n() — the providers, also used by every spec
```

`shared.en.ts` is typed as `Messages` (the type of `shared.de.ts`), and every
`pages/<area>.en.ts` is typed against its German counterpart. **A key without a translation
does not compile** — an untranslated screen is a build failure, not a discovery in production.

In a template:

```html
<h2>{{ 'common.amount' | transloco }}</h2>
```

In TypeScript (a message a request failure produces):

```ts
private readonly i18n = inject(TranslocoService);
this.error.set(this.i18n.translate('kasse.tag.openFailed'));
```

### The language picker

`LanguageService` is the one place the language is decided. It reads the remembered choice
(`localStorage['campus-erp.language']`), applies it before the first render (through
`provideAppInitializer`), mirrors it into `<html lang>`, and loads the dictionary eagerly so the
first paint already has text. The picker offers the languages the product is published in, each
named in its own language ("Deutsch", "English") — a clerk who cannot read the current
interface language still has to find their own.

## Adding a screen

1. Add the route to `app.routes.ts`.
2. Add its entry to `NAV_GROUPS` (`frontend/src/app/ui/nav-model.ts`) — route and label travel
   together; an e2e test fails if the two lists disagree.
3. Add `page.<screen>.title` and `page.<screen>.subtitle` to `shared.de.ts` and `shared.en.ts`.
4. Wrap the template in `<app-page titleKey="page.<screen>.title" subtitleKey="page.<screen>.subtitle">`
   and translate every string — reusing a `shared` key whenever the term already has one. A
   screen that is not a stack of records (the sign-in screen is the only one) takes the `.auth`
   card instead, and says so in its own doc comment.
5. Its own copy goes into the area's `pages/<area>.de.ts` and `.en.ts`.

## Adding a language

1. Write `shared.<lang>.ts` typed as `Messages`, and one `pages/<area>.<lang>.ts` per fragment.
2. Add the language to `SUPPORTED_LANGUAGES` in `i18n/dictionary.ts` and `language.<lang>` to
   the dictionaries ("Deutsch", "English", …).
3. Add it to `DICTIONARIES`. The compiler will name every fragment that is still missing.

## The two guards

`npm run i18n` (run by `make frontend-test`) enforces the two rules that a compiler cannot:

- `scripts/check-copy.mjs` — no component template carries a word; every string is a message key.
- `scripts/check-keys.mjs` — every key a component asks for (`| transloco`, `translate('…')`,
  `titleKey`/`subtitleKey`) exists in the German dictionary, and the parameters agree in both
  directions. The typed dictionaries catch a missing translation, but three things still compile
  and only break on screen: `titleKey="page.haushaltHerkunft"` — one suffix short — renders the key
  itself; `'kasse.orders.approveLabel' | transloco` without `{ reference }` renders
  `{{ reference }}`; and a translation that drops a placeholder shows it in one language only. All
  three are failures, not warnings.

## Tests

Every spec that renders a component with text provides `provideI18n()`:

```ts
providers: [provideHttpClient(), provideHttpClientTesting(), provideI18n()],
```

Assertions read the German text, because German is the default and the offices' language: a
test that expects "Betrag" is also a test that the key exists and the dictionary is shaped
rightly. `app.e2e.spec.ts` additionally switches to English through the picker, checks that the
heading changed and that `<html lang>` followed, and reloads to prove the choice was kept.
