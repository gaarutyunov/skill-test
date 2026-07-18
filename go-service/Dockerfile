# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26.5-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG REVISION=unknown
ARG TAG=""
ARG BUILD_DATE=unknown
ARG REPOSITORY=https://github.com/gaarutyunov/skill-test
ENV SEMCONV=github.com/gaarutyunov/skill-test/go-service/pkg/semconv

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
      -X ${SEMCONV}.Version=${VERSION} \
      -X ${SEMCONV}.Revision=${REVISION} \
      -X ${SEMCONV}.Tag=${TAG} \
      -X ${SEMCONV}.Repository=${REPOSITORY} \
      -X ${SEMCONV}.Date=${BUILD_DATE}" \
    -o /out/report-service .

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/report-service /report-service
COPY --from=build /src/configs /configs
EXPOSE 8080
ENTRYPOINT ["/report-service"]
CMD ["serve", "--config", "/configs/config.yaml"]
