FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend
WORKDIR /src
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/clanboard .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
RUN addgroup -S app && adduser -S -G app app && mkdir -p /data && chown app:app /data
WORKDIR /app
COPY --from=backend /out/clanboard ./clanboard
USER app
ENV PORT=2025 DATABASE_PATH=/data/clan.db
EXPOSE 2025
VOLUME ["/data"]
ENTRYPOINT ["/app/clanboard"]
