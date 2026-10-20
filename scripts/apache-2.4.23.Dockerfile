FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc make libc6-dev libapr1-dev libaprutil1-dev libpcre3-dev liblua5.2-dev zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /tmp
ADD --checksum=sha256:b71a13f56b8061c6b4086fdcc9ffdddd904449735eadec0f0e2947e33eec91d7 \
    https://archive.apache.org/dist/httpd/httpd-2.4.23.tar.gz /tmp/httpd.tar.gz
RUN tar xzf httpd.tar.gz && cd httpd-2.4.23 \
    && ./configure --prefix=/usr/local/apache2 --enable-so --enable-mpms-shared=event --with-mpm=event \
       --enable-lua=shared --with-lua=/usr --enable-proxy=shared --enable-proxy-http=shared \
       --enable-cache=shared --enable-cache-disk=shared --enable-filter=shared --enable-substitute=shared \
       --enable-deflate=shared --enable-headers=shared --enable-mime=shared --enable-alias=shared \
       --enable-authz-core=shared --enable-authz-host=shared --enable-unixd=shared --disable-ssl \
    && make -j2 && make install && rm -rf /tmp/httpd*
ENV PATH=/usr/local/apache2/bin:$PATH
CMD ["httpd", "-DFOREGROUND"]
