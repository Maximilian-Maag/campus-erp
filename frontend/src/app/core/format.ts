/**
 * Presentation helpers shared by the screens.
 *
 * Deliberately not `Intl`: a screen has to render the same figures wherever it runs, and ICU
 * data varies — the same `toLocaleDateString('de-DE')` that gives "01.04.2026" in one browser
 * gives "1.4.2026" in another. German public-finance output is not a place for that.
 */

/** Minor units as a decimal amount ("125000" -> "1.250,00"). */
export function formatCents(cents: number): string {
  const negative = cents < 0;
  const abs = Math.abs(Math.trunc(cents));
  const euro = String(Math.floor(abs / 100)).replace(/\B(?=(\d{3})+(?!\d))/g, '.');
  const rest = String(abs % 100).padStart(2, '0');
  return `${negative ? '-' : ''}${euro},${rest}`;
}

/**
 * A typed amount as minor units ("425,00" -> 42500), or null when it is not a positive amount.
 * The parsing is done on the string, not through a float, so "0,01" is exactly one cent. A comma
 * is accepted as the decimal separator, and thousands dots are tolerated in either style.
 *
 * It sits next to `formatCents` because the two are inverses: every screen that lets an operator
 * type an amount needs this, and a second copy of the rule is how the two directions drift apart.
 */
export function euroToCents(input: string): number | null {
  let raw = input.trim();
  if (raw.includes(',')) {
    raw = raw.replace(/\./g, '').replace(',', '.');
  }
  if (!/^\d{1,14}(\.\d{1,2})?$/.test(raw)) {
    return null;
  }
  const [whole, fraction = ''] = raw.split('.');
  const cents = Number(whole) * 100 + Number(fraction.padEnd(2, '0'));
  return cents > 0 ? cents : null;
}

/**
 * A typed percentage as basis points ("25" -> 2500), or null when it is not one between 0 and
 * 100. Rates and shares are stored as integer basis points (1/100 of a percent) so the arithmetic
 * stays exact, while an operator types percent. The conversion is done on the string, not through
 * a float, so "0,01" is exactly one basis point rather than something that rounds to it. A comma
 * is accepted as the decimal separator.
 *
 * It sits with the cents pair because it is the same idea: what the operator types is not what the
 * API stores, and a second copy of the rule is how the two directions drift apart.
 */
export function percentToBasisPoints(input: string): number | null {
  const raw = input.trim().replace(',', '.');
  if (!/^\d{1,3}(\.\d{1,2})?$/.test(raw)) {
    return null;
  }
  const [whole, fraction = ''] = raw.split('.');
  const basisPoints = Number(whole) * 100 + Number(fraction.padEnd(2, '0'));
  return basisPoints > 10000 ? null : basisPoints;
}

/** The inverse, for showing a stored rate or share: 2500 basis points read as "25 %". */
export function basisPointsToPercent(basisPoints: number): string {
  const whole = Math.trunc(basisPoints / 100);
  const fraction = basisPoints % 100;
  if (fraction === 0) {
    return whole + ' %';
  }
  return whole + ',' + String(fraction).padStart(2, '0').replace(/0$/, '') + ' %';
}

/**
 * A typed quantity as milli-units ("0,5" -> 500), or null when it is not a positive quantity of
 * at most three decimals. Quantities are stored as integer milli-units so the arithmetic stays
 * exact (a line's net is quantity x unit price / 1000), while an operator types units. Like the
 * amount and the rate, the conversion is done on the string rather than through a float, so
 * "0,001" is exactly one milli-unit. A comma is accepted as the decimal separator.
 */
export function quantityToMilli(input: string): number | null {
  const raw = input.trim().replace(',', '.');
  if (!/^\d{1,10}(\.\d{1,3})?$/.test(raw)) {
    return null;
  }
  const [whole, fraction = ''] = raw.split('.');
  const milli = Number(whole) * 1000 + Number(fraction.padEnd(3, '0'));
  return milli > 0 ? milli : null;
}

/** The inverse, for showing a stored quantity: 500 milli-units read as "0,5". */
export function formatMilli(milli: number): string {
  const whole = Math.trunc(milli / 1000);
  const fraction = String(Math.abs(milli) % 1000).padStart(3, '0').replace(/0+$/, '');
  return fraction === '' ? String(whole) : `${whole},${fraction}`;
}

/**
 * A local calendar day as the API reads it ("2026-04-01"), built from the date's own local
 * parts rather than through `toISOString` — which shifts to UTC and so names the previous day
 * east of Greenwich. It is the inverse of `formatDay` and sits with it, because every screen
 * that defaults a period field needs the same day and a second copy is how the two drift apart.
 */
export function isoDay(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${date.getFullYear()}-${month}-${day}`;
}

/**
 * An ISO date as the screen shows it ("2026-04-01T00:00:00Z" -> "01.04.2026"). The date is read
 * in UTC because the API's issue date is a day, not an instant: a local reading would show the
 * previous day west of Greenwich.
 */
export function formatDay(iso: string): string {
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) {
    return iso;
  }
  const day = String(parsed.getUTCDate()).padStart(2, '0');
  const month = String(parsed.getUTCMonth() + 1).padStart(2, '0');
  return `${day}.${month}.${parsed.getUTCFullYear()}`;
}
