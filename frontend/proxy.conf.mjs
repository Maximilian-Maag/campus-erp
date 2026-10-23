/**
 * Dev-server proxy (used by `make workon-frontend` / `npm start -- --proxy-config`).
 *
 * The SPA calls the API on the same origin under /api; in `ng serve` there is no
 * nginx in front of it, so forward /api to a backend — either one deployed
 * elsewhere (API_PROXY_TARGET, e.g. https://uni-x.erp.example.org) or a locally
 * running api (the default). This is what lets you work on the frontend against a
 * deployed backend.
 */
const target = process.env.API_PROXY_TARGET || 'http://localhost:8080';

export default {
  '/api': {
    target,
    changeOrigin: true,
    secure: false,
    logLevel: 'info',
  },
};
