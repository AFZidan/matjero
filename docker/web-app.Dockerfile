FROM node:24-alpine AS build

ENV NEXT_TELEMETRY_DISABLED=1

WORKDIR /src
COPY package.json package-lock.json ./
COPY web/admin/package.json web/admin/package.json
COPY web/seller/package.json web/seller/package.json
COPY web/supplier/package.json web/supplier/package.json
COPY web/storefront/package.json web/storefront/package.json
RUN npm ci

COPY web ./web
COPY scripts ./scripts

ARG WORKSPACE=@commerce/admin-web
RUN npm run build --workspace ${WORKSPACE}
