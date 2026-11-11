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

/**
 * The types a **run** may be filed under: the four SEPA types plus "each mandate's own".
 *
 * The extra option exists because the sequence type belongs to the payment instruction, not to the
 * message: a run filed without one collects every mandate under its own type and the file carries one
 * instruction per type. It is not a type a mandate can carry, which is why it is not in
 * SEQUENCE_TYPES — the mandate screens must never offer it.
 */
export const RUN_SEQUENCE_TYPES: readonly SequenceType[] = [
  { value: '', label: 'je Mandat' },
  ...SEQUENCE_TYPES,
];

/**
 * The word for the type a **run** collected under. A run with no single type is *gemischt*: that is a
 * fact about the produced file rather than the choice made in the form, which is why the option reads
 * "je Mandat" and the result reads "gemischt".
 */
export function runSequenceLabel(value: string): string {
  return value === '' ? 'gemischt' : sequenceLabel(value);
}
