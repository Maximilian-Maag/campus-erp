/**
 * The SEPA direct-debit vocabulary two screens share (docs/20, docs/21).
 *
 * A **sequence type** is what a collection run gathers and what a mandate is registered under, and
 * the two must agree: a run that collects `RCUR` can only gather mandates that carry `RCUR`. The
 * list therefore lives here rather than in each screen, so the two cannot drift apart — and the
 * server's own codes are the values, with the German word only a label.
 */
export interface SequenceType {
  /** The code the API carries (ISO 20022 `SeqTp`: FRST / RCUR / OOFF / FNAL). */
  value: string;
  /** The word the screen shows, since "RCUR" is not a word. */
  label: string;
}

/** The four sequence types, in the order a mandate's life runs through them. */
export const SEQUENCE_TYPES: readonly SequenceType[] = [
  { value: 'FRST', label: 'erstmalig' },
  { value: 'RCUR', label: 'wiederkehrend' },
  { value: 'OOFF', label: 'einmalig' },
  { value: 'FNAL', label: 'letztmalig' },
];

/**
 * The German word for a sequence type, or the code itself when it is one this build does not know:
 * a value the server accepted must be shown as it is, not blanked out because the label is missing.
 */
export function sequenceLabel(value: string): string {
  return SEQUENCE_TYPES.find((type) => type.value === value)?.label ?? value;
}
