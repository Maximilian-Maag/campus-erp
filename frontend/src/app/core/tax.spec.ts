import { VAT_KINDS, vatKindLabel } from './tax';

describe('VAT_KINDS and vatKindLabel', () => {
  it('names the two kinds the API carries', () => {
    expect(VAT_KINDS.map((k) => k.value)).toEqual(['output', 'input']);
    expect(vatKindLabel('output')).toBe('Umsatzsteuer');
    expect(vatKindLabel('input')).toBe('Vorsteuer');
  });

  it('shows an unknown kind as its own code rather than as nothing', () => {
    expect(vatKindLabel('gemischt')).toBe('gemischt');
    expect(vatKindLabel('')).toBe('');
  });
});
