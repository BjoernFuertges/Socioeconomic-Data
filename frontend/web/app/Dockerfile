# =============================================================================
# Stage 1: Build der Vue-SPA
# =============================================================================
FROM node:22-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# =============================================================================
# Stage 2: Minimaler Static-File-Server
# =============================================================================
FROM nginx:1.27-alpine

COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
