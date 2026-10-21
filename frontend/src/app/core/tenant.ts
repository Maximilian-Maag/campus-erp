/**
 * Tenant resolution (ADR-0002).
 *
 * The tenant is the first label of the hostname, e.g.
 *   "uni-x.erp.example.org" -> "uni-x"
 * The API is called on the same origin, so the browser does not carry the tenant
 * in the path or a query parameter.
 */

const LABEL = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;

/** Extract the tenant from a hostname. Throws on an unusable host. */
export function tenantFromHostname(host: string): string {
  const h = host.trim().toLowerCase();
  if (h === '') {
    throw new Error('tenant: empty hostname');
  }
  const hostname = h.split(':')[0];
  const label = hostname.split('.')[0];
  if (!LABEL.test(label)) {
    throw new Error(`tenant: invalid hostname "${host}"`);
  }
  return label;
}

/** The API base URL for a tenant (same origin by default). */
export function apiBaseFor(origin: string): string {
  const trimmed = origin.replace(/\/+$/, '');
  if (trimmed === '') {
    throw new Error('tenant: empty origin');
  }
  return `${trimmed}/api`;
}
