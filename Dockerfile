# cgr.dev/chainguard/git:latest
# Git is required for repository discovery and wiki readiness checks.
FROM cgr.dev/chainguard/git@sha256:42eb72ec1720b0703220d1a7cd3ced2b360659a6cbc94898187faee4469d4c47
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/tailor /usr/local/bin/tailor
USER 65532
ENTRYPOINT ["tailor"]
