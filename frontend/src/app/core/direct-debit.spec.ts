import { RUN_SEQUENCE_TYPES, SEQUENCE_TYPES, runSequenceKey, sequenceKey } from './direct-debit';

describe('SEQUENCE_TYPES and sequenceKey', () => {
  it('lists the four sequence types with their codes', () => {
    expect(SEQUENCE_TYPES.map((t) => t.value)).toEqual(['FRST', 'RCUR', 'OOFF', 'FNAL']);
    expect(SEQUENCE_TYPES.every((t) => t.labelKey.startsWith('sepa.sequence.'))).toBeTrue();
  });

  it('names a sequence type by its message key', () => {
    expect(sequenceKey('RCUR')).toBe('sepa.sequence.recurring');
    expect(sequenceKey('FNAL')).toBe('sepa.sequence.final');
  });

  it('shows an unknown sequence type as its own code rather than as nothing', () => {
    expect(sequenceKey('XXXX')).toBe('XXXX');
    expect(sequenceKey('')).toBe('');
  });
});

describe('RUN_SEQUENCE_TYPES and runSequenceKey', () => {
  it('adds the extra choice a run may be filed under, first and alone', () => {
    expect(RUN_SEQUENCE_TYPES.map((t) => t.value)).toEqual(['', 'FRST', 'RCUR', 'OOFF', 'FNAL']);
    // The mandate list must not offer it: a mandate cannot be "each mandate's own".
    expect(SEQUENCE_TYPES.map((t) => t.value)).not.toContain('');
  });

  it('words a run with no single type as one whose types are mixed', () => {
    expect(runSequenceKey('')).toBe('sepa.mixed');
    expect(runSequenceKey('FRST')).toBe('sepa.sequence.first');
    // An unknown type is still shown as itself.
    expect(runSequenceKey('XX')).toBe('XX');
  });

  it('carries the four SEPA types through, so a run form offers every one of them', () => {
    for (const t of SEQUENCE_TYPES) {
      expect(runSequenceKey(t.value)).toBe(t.labelKey);
    }
    expect(RUN_SEQUENCE_TYPES.every((t) => t.labelKey !== '')).toBeTrue();
  });
});
