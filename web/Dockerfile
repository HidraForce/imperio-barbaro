FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
RUN addgroup -S crm && adduser -S -G crm crm
WORKDIR /app
COPY --from=build /app/dist ./dist
COPY gateway.mjs server.mjs package.json ./
USER crm
ENV PORT=3000
EXPOSE 3000
CMD ["node", "server.mjs"]
