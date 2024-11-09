# Support setting various labels on the final image
ARG COMMIT=""
ARG VERSION=""
ARG BUILDNUM=""

# Build ETN-SC in a stock Go builder container
FROM golang:1.23-alpine3.20@sha256:d0b31558e6b3e4cc59f6011d79905835108c919143ebecc58f35965bf79948f4 AS builder

RUN apk add --no-cache gcc musl-dev linux-headers git

# Get dependencies - will also be cached if we won't change go.mod/go.sum
COPY go.mod /electroneum-sc/
COPY go.sum /electroneum-sc/
RUN cd /electroneum-sc && go mod download

ADD . /electroneum-sc
RUN cd /electroneum-sc && go run build/ci.go install ./cmd/etn-sc

# Pull ETN-SC into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates bash
COPY --from=builder /electroneum-sc/build/bin/etn-sc /usr/local/bin/

EXPOSE 8545 8546 30303 30303/udp
ENTRYPOINT ["etn-sc"]

# Add some metadata labels to help programatic image consumption
ARG COMMIT=""
ARG VERSION=""
ARG BUILDNUM=""

LABEL commit="$COMMIT" version="$VERSION" buildnum="$BUILDNUM"
