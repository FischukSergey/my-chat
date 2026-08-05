# Сборка PWA (Vite) и упаковка статики в nginx-образ.
# Конфиг nginx монтируется volume'ами из deploy/prod/nginx/.

FROM node:22-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# Same-origin: пустые VITE_* → relative /api, /ws (nginx проксирует).
RUN npm run build

FROM nginx:1.27-alpine

COPY --from=build /app/dist /usr/share/nginx/html
