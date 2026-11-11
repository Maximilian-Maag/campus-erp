/**
 * The tax vocabulary a screen shows (docs/07, `VATKind` in Kasse).
 *
 * A VAT type is either **Umsatzsteuer** (the tax this body charges) or **Vorsteuer** (the tax it
 * paid and reclaims). The API carries the two as `output`/`input`; the German word is only a label,
 * and an unknown kind is shown as its own code rather than blanked out.
 */
export interface VATKind {
  /** The code the API carries. */
  value: string;
  /** The word the screen shows. */
  label: string;
}

export const VAT_KINDS: readonly VATKind[] = [
  { value: 'output', label: 'Umsatzsteuer' },
  { value: 'input', label: 'Vorsteuer' },
];

/** The German word for a kind, or the code itself when it is one this build does not know. */
export function vatKindLabel(value: string): string {
  return VAT_KINDS.find((kind) => kind.value === value)?.label ?? value;
}
