# syntax=docker/dockerfile:1
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
# NEXT_PUBLIC_* is inlined at build time; API_INTERNAL_URL is baked into rewrites at build time.
ARG API_INTERNAL_URL=http://api:8080
ARG NEXT_PUBLIC_MOCK_API=false
ARG NEXT_PUBLIC_MCP_URL=http://localhost:3333/mcp
ARG NEXT_PUBLIC_API_URL=http://localhost:8080
ENV API_INTERNAL_URL=$API_INTERNAL_URL NEXT_PUBLIC_MOCK_API=$NEXT_PUBLIC_MOCK_API \
    NEXT_PUBLIC_MCP_URL=$NEXT_PUBLIC_MCP_URL NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
RUN addgroup -S app && adduser -S app -G app
COPY --from=build --chown=app:app /app/.next/standalone ./
COPY --from=build --chown=app:app /app/.next/static ./.next/static
COPY --from=build --chown=app:app /app/public ./public
USER app
EXPOSE 3000
CMD ["node", "server.js"]
