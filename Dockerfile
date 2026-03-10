FROM cgr.dev/chainguard/static:latest
COPY tailor /usr/local/bin/tailor
ENTRYPOINT ["tailor"]
