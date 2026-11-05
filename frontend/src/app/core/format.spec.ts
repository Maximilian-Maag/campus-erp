import {
  basisPointsToPercent,
  formatCents,
  formatDay,
  percentToBasisPoints,
} from './format';

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

describe('percentToBasisPoints', () => {
  it('reads a whole percentage', () => {
    expect(percentToBasisPoints('25')).toBe(2500);
    expect(percentToBasisPoints('100')).toBe(10000);
    expect(percentToBasisPoints('0')).toBe(0);
  });

  it('reads one and two decimals, with a dot or a comma', () => {
    expect(percentToBasisPoints('12.5')).toBe(1250);
    expect(percentToBasisPoints('12,5')).toBe(1250);
    expect(percentToBasisPoints('0.01')).toBe(1);
    expect(percentToBasisPoints('33,33')).toBe(3333);
  });

  it('tolerates surrounding spaces', () => {
    expect(percentToBasisPoints('  7  ')).toBe(700);
  });

  it('refuses what is not a percentage between 0 and 100', () => {
    expect(percentToBasisPoints('')).toBeNull();
    expect(percentToBasisPoints('abc')).toBeNull();
    expect(percentToBasisPoints('-5')).toBeNull();
    expect(percentToBasisPoints('100.5')).toBeNull();
    expect(percentToBasisPoints('12.345')).toBeNull();
    expect(percentToBasisPoints('1234')).toBeNull();
    expect(percentToBasisPoints('1.2.3')).toBeNull();
  });
});

describe('basisPointsToPercent', () => {
  it('renders whole percentages without a fraction', () => {
    expect(basisPointsToPercent(2500)).toBe('25 %');
    expect(basisPointsToPercent(10000)).toBe('100 %');
    expect(basisPointsToPercent(0)).toBe('0 %');
  });

  it('renders one decimal where it is needed', () => {
    expect(basisPointsToPercent(1250)).toBe('12,5 %');
    expect(basisPointsToPercent(2550)).toBe('25,5 %');
  });

  it('renders a single basis point as two decimals', () => {
    expect(basisPointsToPercent(1)).toBe('0,01 %');
    expect(basisPointsToPercent(3333)).toBe('33,33 %');
  });

  it('round-trips with percentToBasisPoints', () => {
    for (const typed of ['0,01', '12,5', '33,33', '100']) {
      const bp = percentToBasisPoints(typed);
      expect(bp).not.toBeNull();
      expect(percentToBasisPoints(basisPointsToPercent(bp as number).replace(' %', ''))).toBe(bp);
    }
  });
});
