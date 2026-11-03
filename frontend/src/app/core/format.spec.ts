import { formatCents, formatDay } from './format';

describe('formatCents', () => {
  it('renders minor units as a German amount', () => {
    expect(formatCents(0)).toBe('0,00');
    expect(formatCents(5)).toBe('0,05');
    expect(formatCents(100)).toBe('1,00');
    expect(formatCents(35700)).toBe('357,00');
    expect(formatCents(125000)).toBe('1.250,00');
    expect(formatCents(123456789)).toBe('1.234.567,89');
  });

  it('keeps a negative amount negative, and does not carry the sign into the grouping', () => {
    expect(formatCents(-250)).toBe('-2,50');
    expect(formatCents(-1234567)).toBe('-12.345,67');
  });

  it('truncates a fraction of a cent rather than rounding the screen', () => {
    expect(formatCents(1234.9)).toBe('12,34');
  });
});

describe('formatDay', () => {
  it('renders an ISO date as a German day with leading zeros', () => {
    expect(formatDay('2026-04-01T00:00:00Z')).toBe('01.04.2026');
    expect(formatDay('2026-12-31T23:59:59Z')).toBe('31.12.2026');
  });

  it('reads the day in UTC, so the day does not shift with the machine zone', () => {
    expect(formatDay('2026-04-01T00:30:00Z')).toBe('01.04.2026');
    expect(formatDay('2026-04-01T23:30:00Z')).toBe('01.04.2026');
  });

  it('shows a value it cannot parse as it came', () => {
    expect(formatDay('nonsense')).toBe('nonsense');
    expect(formatDay('')).toBe('');
  });
});
