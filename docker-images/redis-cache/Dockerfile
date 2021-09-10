FROM redis:5-alpine@sha256:fea243676a4d2d67f5990ddcbd4a56db9423b7f25e55758491e39988efc1cfbe

RUN mkdir -p /redis-data && chown -R redis:redis /redis-data
# hadolint ignore=DL3018
RUN apk update --no-cache && apk upgrade --no-cache && \
    apk --no-cache add tini

USER redis
COPY redis.conf /etc/redis/redis.conf

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["redis-server", "/etc/redis/redis.conf"]
