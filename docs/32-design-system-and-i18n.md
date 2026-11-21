# 32 — The design system and the two languages

The rebuild's interface was twenty-five hand-written screens that each invented their own
markup and wrote their own German copy. That produced a product that looked like five
products and called the same thing by three names. This document is the rule that stops it
from happening again: **a screen carries structure and meaning; the design system carries the
look; the dictionaries carry the words.**

## The look: Angular Material, one theme

Everything visual comes from Angular Material's Material 3 theme, set once in
`frontend/src/styles.scss`:

```scss
html {
  color-scheme: light dark;
  @include mat.theme((color: (primary: mat.$blue-palette, tertiary: mat.$cyan-palette), …));
}
```

That call emits the `--mat-sys-*` tokens (colour roles, the type scale, elevation, shape) and
`color-scheme: light dark` makes them resolve to the light or the dark set the operating system
asks for. Re-theming Campus ERP means editing that one call — not twenty stylesheets.

Three layers sit on top of it:

1. **The global stylesheet** (`styles.scss`) styles the element, not the screen: a `<section>`
   is a card, a `<table>` is a dense Material table with a sticky header and tabular numerals,
   a `<button>` is the product's filled action, `input`/`select` are outlined fields, `.form` is
   a responsive grid of `.field`s, `.error` is an error container. Because the screens were
   already written as `<section>` + `<table>` + `.form`, they inherit the design without being
   rewritten.
2. **Shared components** in `frontend/src/app/ui/`:
   - `app-shell` — the frame: application bar with the product name and the language picker, a
     grouped navigation panel (permanent on a wide screen, an overlay that closes on a narrow
     one), and the content area.
   - `app-page` — the head of a screen: `<app-page titleKey="page.…" subtitleKey="page.…">`
     renders the `<h1>`, the one-sentence description, and a slot for the screen's action.
   - `app-language-picker` — the picker.
3. **No component stylesheets for looks.** A component may style its own layout
   (`.page-head` in `app-page`), never a colour, a font size or a radius: those are tokens.

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
   and translate every string — reusing a `shared` key whenever the term already has one.
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
  `titleKey`/`subtitleKey`) exists in the German dictionary. The typed dictionaries catch a missing
  translation, but `titleKey="page.haushaltHerkunft"` — one suffix short — compiles and renders the
  key itself; this is what catches that.

## Tests

Every spec that renders a component with text provides `provideI18n()`:

```ts
providers: [provideHttpClient(), provideHttpClientTesting(), provideI18n()],
```

Assertions read the German text, because German is the default and the offices' language: a
test that expects "Betrag" is also a test that the key exists and the dictionary is shaped
rightly. `app.e2e.spec.ts` additionally switches to English through the picker, checks that the
heading changed and that `<html lang>` followed, and reloads to prove the choice was kept.
