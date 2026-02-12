# Stage 1: Build Vue frontend
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.25-alpine AS backend
RUN apk add --no-cache gcc musl-dev libcap
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy built frontend into embed directory
COPY --from=frontend /app/frontend/dist cmd/bekci/frontend_dist/
RUN CGO_ENABLED=1 go build -ldflags "-X main.version=2.0.0" -o /bekci ./cmd/bekci
RUN setcap cap_net_raw+ep /bekci

# Stage 3: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates libcap
COPY --from=backend /bekci /usr/local/bin/bekci
RUN mkdir -p /data
WORKDIR /data
EXPOSE 65000
ENTRYPOINT ["bekci"]
