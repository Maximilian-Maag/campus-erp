import { tenantFromHostname, apiBaseFor } from './tenant';

describe('tenantFromHostname', () => {
  it('takes the first label of the hostname', () => {
    expect(tenantFromHostname('uni-x.erp.example.org')).toBe('uni-x');
  });

  it('is case-insensitive', () => {
    expect(tenantFromHostname('UNI-X.Erp.Example.ORG')).toBe('uni-x');
  });

  it('ignores surrounding whitespace', () => {
    expect(tenantFromHostname('  uni-x.example.org  ')).toBe('uni-x');
  });

  it('ignores a port', () => {
    expect(tenantFromHostname('uni-x.erp.example.org:8080')).toBe('uni-x');
  });

  it('accepts a single-character label', () => {
    expect(tenantFromHostname('a')).toBe('a');
  });

  it('accepts a bare label', () => {
    expect(tenantFromHostname('demo')).toBe('demo');
  });

  it('accepts a 63-character label', () => {
    expect(tenantFromHostname('a'.repeat(63))).toBe('a'.repeat(63));
  });

  it('rejects an empty hostname', () => {
    expect(() => tenantFromHostname('')).toThrowError(/empty hostname/);
  });

  it('rejects a whitespace-only hostname', () => {
    expect(() => tenantFromHostname('   ')).toThrowError(/empty hostname/);
  });

  it('rejects a label with invalid characters', () => {
    expect(() => tenantFromHostname('uni_x.example.org')).toThrowError(/invalid hostname/);
  });

  it('rejects a label with a leading dash', () => {
    expect(() => tenantFromHostname('-bad.example.org')).toThrowError(/invalid hostname/);
  });

  it('rejects a label with a trailing dash', () => {
    expect(() => tenantFromHostname('bad-.example.org')).toThrowError(/invalid hostname/);
  });
});

describe('apiBaseFor', () => {
  it('appends /api', () => {
    expect(apiBaseFor('https://uni-x.example.org')).toBe('https://uni-x.example.org/api');
  });

  it('drops a single trailing slash', () => {
    expect(apiBaseFor('https://uni-x.example.org/')).toBe('https://uni-x.example.org/api');
  });

  it('drops multiple trailing slashes', () => {
    expect(apiBaseFor('https://uni-x.example.org///')).toBe('https://uni-x.example.org/api');
  });

  it('rejects an empty origin', () => {
    expect(() => apiBaseFor('')).toThrowError(/empty origin/);
  });

  it('rejects a slash-only origin', () => {
    expect(() => apiBaseFor('///')).toThrowError(/empty origin/);
  });
});
