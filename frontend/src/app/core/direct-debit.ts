/**
 * The SEPA direct-debit vocabulary two screens share (docs/20, docs/21).
 *
 * A **sequence type** is what a collection run gathers and what a mandate is registered under, and
 * the two must agree: a run that collects `RCUR` can only gather mandates that carry `RCUR`. The
 * list therefore lives here rather than in each screen, so the two cannot drift apart — and the
 * server's own codes are the values, with the word a message key (`sepa.sequence.*`) rather than a
 * string: a label held here could not be translated.
 */
export interface SequenceType {
  /** The code the API carries (ISO 20022 `SeqTp`: FRST / RCUR / OOFF / FNAL). */
  value: string;
  /** The message key of the word the screen shows, since "RCUR" is not a word. */
  labelKey: string;
}

/** The four sequence types, in the order a mandate's life runs through them. */
export const SEQUENCE_TYPES: readonly SequenceType[] = [
  { value: 'FRST', labelKey: 'sepa.sequence.first' },
  { value: 'RCUR', labelKey: 'sepa.sequence.recurring' },
  { value: 'OOFF', labelKey: 'sepa.sequence.oneOff' },
  { value: 'FNAL', labelKey: 'sepa.sequence.final' },
];

/**
 * The message key of a sequence type, or the code itself when it is one this build does not know:
 * a value the server accepted must be shown as it is, not blanked out because the label is missing.
 */
export function sequenceKey(value: string): string {
  return SEQUENCE_TYPES.find((type) => type.value === value)?.labelKey ?? value;
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
  { value: '', labelKey: 'sepa.perMandate' },
  ...SEQUENCE_TYPES,
];

/**
 * The key of the word for the type a **run** collected under. A run with no single type is *mixed*:
 * a fact about the produced file rather than the choice made in the form, which is why the option
 * reads "each mandate's own" and the result reads "mixed".
 */
export function runSequenceKey(value: string): string {
  return value === '' ? 'sepa.mixed' : sequenceKey(value);
}
