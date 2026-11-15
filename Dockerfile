FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /terraview .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /terraview /terraview
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/terraview"]
