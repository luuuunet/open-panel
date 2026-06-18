FROM golang:1.22-alpine AS backend-builder
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /owpanel ./cmd/server

FROM node:20-alpine AS frontend-builder
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npx vite build --outDir dist

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /opt/owpanel
COPY --from=backend-builder /owpanel .
COPY --from=frontend-builder /src/dist ./web
ENV OWPANEL_PORT=8888
ENV OWPANEL_DATA=/opt/owpanel/data
ENV OWPANEL_WEB=/opt/owpanel/web
EXPOSE 8888
VOLUME ["/opt/owpanel/data"]
ENTRYPOINT ["./owpanel"]
