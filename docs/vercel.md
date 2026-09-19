# Vercel frontend and local Docker backend

The Vercel project `metamax` is connected to `shreejaykurhade/MetaMax`, with
`frontend` as its root directory and Node.js 22. Production deploys use `main`.

The Go API, PostgreSQL, and Docker-in-Docker run on the local computer. Start the
temporary HTTPS tunnel with:

```powershell
docker compose -f docker-compose.yml -f compose.tunnel.yml up -d
docker compose -f docker-compose.yml -f compose.tunnel.yml logs tunnel
```

Set Vercel's production `BACKEND_INTERNAL_URL` to the HTTPS URL shown in the
tunnel log, and `NEXT_PUBLIC_WS_URL` to the same host using `wss://`. Redeploy
after changing either value. HTTP requests use the frontend's `/api/backend`
rewrite; WebSocket connections go directly to the tunnel. The Groq key belongs
only in the local backend `.env`, never in a public frontend variable.

Quick Tunnel URLs may change on restart. This setup requires the computer,
Docker, internet connection, and tunnel to remain running. Use a named tunnel
with a stable hostname or a dedicated server for reliable production use.

## Domain and certificates

`meta-max.xyz`, `www.meta-max.xyz`, and `*.meta-max.xyz` are assigned to Vercel.
For Vercel-managed wildcard certificates, set the registrar's nameservers to:

- `ns1.vercel-dns.com`
- `ns2.vercel-dns.com`

Copy existing mail and other DNS records into Vercel before switching providers.
Vercel can issue certificates after DNS and domain ownership verification pass.
Wildcard assignment currently serves the frontend on those hostnames; it does
not yet route individual subdomains to Docker workloads. That requires a stable
backend ingress and separate workload routing configuration.

Blockchain payments, provider registration, and attestations still require
their contract addresses and wallet configuration.
