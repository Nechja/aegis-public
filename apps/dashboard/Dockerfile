FROM node:22-alpine AS frontend
ARG COMMIT_SHA=dev
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /app
COPY frontend/package.json frontend/pnpm-lock.yaml ./
ENV PNPM_ALLOW_BUILDS=true
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN VITE_COMMIT_SHA=${COMMIT_SHA} pnpm build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /app/build ./internal/server/frontend/build
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
