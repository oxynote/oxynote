FROM gcr.io/distroless/static-debian10:nonroot
ARG BINARY
COPY ${BINARY} /app
ENTRYPOINT ["/app"]
EXPOSE 8080
