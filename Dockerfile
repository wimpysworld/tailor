FROM cgr.dev/chainguard/static:latest
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/tailor /usr/local/bin/tailor
ENTRYPOINT ["tailor"]
