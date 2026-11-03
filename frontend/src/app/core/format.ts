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
