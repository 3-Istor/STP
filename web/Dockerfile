# Image du front pour la stack compose (voir ADR 0005) : build Vite, puis fichiers statiques servis par nginx.
# Images de base épinglées : on les monte de version volontairement, jamais `latest`.

FROM node:24.11.1-alpine3.23 AS build
WORKDIR /app
# Dépendances d'abord : la couche reste en cache tant que le lockfile ne change pas.
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.29.4-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
