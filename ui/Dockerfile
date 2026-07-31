FROM node:22-alpine AS builder
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM caddy:2.8.4-alpine
COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=builder /src/dist /srv
