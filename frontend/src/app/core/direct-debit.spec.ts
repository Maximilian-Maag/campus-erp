import { RUN_SEQUENCE_TYPES, SEQUENCE_TYPES, runSequenceLabel, sequenceLabel } from './direct-debit';

describe('SEQUENCE_TYPES and sequenceLabel', () => {
  it('lists the four sequence types with their codes', () => {
    expect(SEQUENCE_TYPES.map((t) => t.value)).toEqual(['FRST', 'RCUR', 'OOFF', 'FNAL']);
    expect(SEQUENCE_TYPES.every((t) => t.label !== '')).toBeTrue();
  });

  it('names a sequence type in German', () => {
    expect(sequenceLabel('RCUR')).toBe('wiederkehrend');
    expect(sequenceLabel('FNAL')).toBe('letztmalig');
  });

  it('shows an unknown sequence type as its own code rather than as nothing', () => {
    expect(sequenceLabel('XXXX')).toBe('XXXX');
    expect(sequenceLabel('')).toBe('');
  });
});

describe('RUN_SEQUENCE_TYPES and runSequenceLabel', () => {
  it('adds the extra choice a run may be filed under, first and alone', () => {
    expect(RUN_SEQUENCE_TYPES.map((t) => t.value)).toEqual(['', 'FRST', 'RCUR', 'OOFF', 'FNAL']);
    // The mandate list must not offer it: a mandate cannot be "each mandate's own".
    expect(SEQUENCE_TYPES.map((t) => t.value)).not.toContain('');
  });

  it('words a run with no single type as one whose types are mixed', () => {
    expect(runSequenceLabel('')).toBe('gemischt');
    expect(runSequenceLabel('FRST')).toBe('erstmalig');
    // An unknown type is still shown as itself.
    expect(runSequenceLabel('XX')).toBe('XX');
  });

  it('carries the four SEPA types through, so a run form offers every one of them', () => {
    for (const t of SEQUENCE_TYPES) {
      expect(runSequenceLabel(t.value)).toBe(t.label);
    }
    expect(RUN_SEQUENCE_TYPES.every((t) => t.label !== '')).toBeTrue();
  });
});
