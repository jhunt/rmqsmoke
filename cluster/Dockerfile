FROM haproxy
RUN apt-get update \
 && apt-get install -y python curl \
 && curl -Lo /usr/bin/hatop https://github.com/jhunt/hatop/releases/download/v0.8.0/hatop \
 && chmod 0755 /usr/bin/hatop
