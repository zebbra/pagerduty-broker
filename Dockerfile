FROM alpine

LABEL org.opencontainers.image.source = "https://github.com/zebbra/pagerduty-broker"
LABEL org.opencontainers.image.license = "MIT"

COPY pagerduty-broker /pagerduty-broker
ENTRYPOINT ["/pagerduty-broker"]
