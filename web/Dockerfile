FROM node:22-alpine AS build
WORKDIR /web

# `npm ci` exige lockfile y lo respeta exactamente. `npm install` resolvia
# rangos en cada build: dos builds del mismo commit podian traer dependencias
# distintas.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /web/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
