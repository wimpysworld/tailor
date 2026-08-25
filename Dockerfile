# cgr.dev/chainguard/static:latest
FROM cgr.dev/chainguard/static@sha256:f68e3a8244c7d0f4cd56635aaff8e6a533cf6cc3850d8fb339567a5782d6a0b0
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/tailor /usr/local/bin/tailor
USER 65532
ENTRYPOINT ["tailor"]
