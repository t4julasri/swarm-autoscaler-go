FROM --platform=linux/amd64 ubuntu:jammy

# Install Docker and dependencies
RUN apt-get update -qq && \
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  jq \
  apt-transport-https \
  ca-certificates \
  curl \
  gnupg \
  lsb-release \
  software-properties-common \
  dnsutils && \
  install -m 0755 -d /etc/apt/keyrings && \
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
  chmod a+r /etc/apt/keyrings/docker.gpg && \
  echo \
  "deb [arch="$(dpkg --print-architecture)" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  "$(. /etc/os-release && echo "$VERSION_CODENAME")" stable" | \
  tee /etc/apt/sources.list.d/docker.list > /dev/null && \
  apt-get update -qq && \
  apt-get install -y --no-install-recommends \
  docker-ce \
  docker-ce-cli \
  containerd.io && \
  apt-get clean && \
  apt-get autoremove -y && \
  rm -rf \
  /var/lib/apt/lists/* \
  /tmp/* \
  /var/tmp/*

COPY auto-scale.sh /auto-scale.sh
RUN chmod a+x /auto-scale.sh

CMD ["/auto-scale.sh"]