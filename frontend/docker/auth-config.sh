#!/bin/sh
# `sh`, not `bash` as AGENTS.md otherwise asks: this runs inside nginx:alpine before it
# serves its first request, and that image has busybox sh and no bash to point a shebang at.
#
# Writes the interface's sign-in configuration from the container environment.
#
# The image is built once and deployed many times (test IdP, production IdP), so the issuer
# and client id cannot be baked into the bundle at build time. nginx's entrypoint runs every
# `/docker-entrypoint.d/*.sh` before it starts, which makes this the writing-before-serving
# hook.
#
# An empty ERP_OIDC_ISSUER writes a configuration with sign-in off: the interface then runs
# unauthenticated, exactly like the local compose stack.
set -eu

target=/usr/share/nginx/html/config.json

# JSON-escaping for the four values: backslashes and quotes are the only characters that
# cannot appear in an issuer URL or a client id, and they are escaped rather than rejected.
escape() {
    printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

issuer=$(escape "${ERP_OIDC_ISSUER:-}")
client_id=$(escape "${ERP_OIDC_CLIENT_ID:-}")
scope=$(escape "${ERP_OIDC_SCOPE:-openid profile email}")
redirect_uri=$(escape "${ERP_OIDC_REDIRECT_URI:-}")

cat > "$target" <<EOF
{"auth":{"issuer":"$issuer","clientId":"$client_id","scope":"$scope","redirectUri":"$redirect_uri"}}
EOF

echo "campus-erp: sign-in $([ -n "$issuer" ] && echo "configured for $issuer" || echo disabled)"