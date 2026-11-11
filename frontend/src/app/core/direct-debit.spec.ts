import { SEQUENCE_TYPES, sequenceLabel } from './direct-debit';

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
