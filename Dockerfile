FROM codefresh/cli
LABEL maintainer="tech-ally@lacework.net" \
      description="Your release ally (Slack App)"

COPY startup/ally.toml /cf-cli
ADD bin/ally-linux-amd64 /usr/local/bin/ally
RUN rm -f /root/.cfconfig

# Install Github CLI
RUN apk add github-cli

ENTRYPOINT ["/usr/local/bin/ally"]
