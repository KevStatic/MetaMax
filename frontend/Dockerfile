FROM node:22-alpine AS deps

# utf-8-validate / bufferutil are native addons built by node-gyp
RUN apk add --no-cache python3 make g++

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev

# ── Build ──────────────────────────────────────────────────────────────────

FROM node:22-alpine AS builder

RUN apk add --no-cache python3 make g++

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

ARG BACKEND_INTERNAL_URL=http://backend:8080
ENV BACKEND_INTERNAL_URL=$BACKEND_INTERNAL_URL
ARG NEXT_PUBLIC_PROVIDER_REGISTRY_ADDRESS
ARG NEXT_PUBLIC_DEPLOYMENT_ESCROW_ADDRESS
ARG NEXT_PUBLIC_JOB_AUCTION_ADDRESS
ARG NEXT_PUBLIC_EXECUTION_ATTESTATION_ADDRESS
ARG NEXT_PUBLIC_MONAD_TESTNET_RPC_URL
ENV NEXT_PUBLIC_PROVIDER_REGISTRY_ADDRESS=$NEXT_PUBLIC_PROVIDER_REGISTRY_ADDRESS
ENV NEXT_PUBLIC_DEPLOYMENT_ESCROW_ADDRESS=$NEXT_PUBLIC_DEPLOYMENT_ESCROW_ADDRESS
ENV NEXT_PUBLIC_JOB_AUCTION_ADDRESS=$NEXT_PUBLIC_JOB_AUCTION_ADDRESS
ENV NEXT_PUBLIC_EXECUTION_ATTESTATION_ADDRESS=$NEXT_PUBLIC_EXECUTION_ATTESTATION_ADDRESS
ENV NEXT_PUBLIC_MONAD_TESTNET_RPC_URL=$NEXT_PUBLIC_MONAD_TESTNET_RPC_URL
ARG NEXT_PUBLIC_GITHUB_CLIENT_ID
ENV NEXT_PUBLIC_GITHUB_CLIENT_ID=$NEXT_PUBLIC_GITHUB_CLIENT_ID
ARG NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID

ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID=$NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID
ENV NEXT_TELEMETRY_DISABLED=1

RUN mkdir -p public && npm run build

# ── Runtime ────────────────────────────────────────────────────────────────

FROM node:22-alpine AS runner

WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 nodejs && \
    adduser  --system --uid 1001 nextjs

COPY --from=builder /app/public       ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

CMD ["node", "server.js"]
