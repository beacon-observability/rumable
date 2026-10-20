FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc make libc6-dev libpcre3-dev zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /tmp
ADD --checksum=sha256:479b0c03747ee6b2d4a21046f89b06d178a2881ea80cfef160451325788f2ba8 \
    https://nginx.org/download/nginx-1.9.4.tar.gz /tmp/nginx-1.9.4.tar.gz
RUN tar xzf nginx-1.9.4.tar.gz \
    && cd nginx-1.9.4 \
    && ./configure --prefix=/etc/nginx --sbin-path=/usr/sbin/nginx --conf-path=/etc/nginx/nginx.conf \
       --with-http_sub_module --with-http_gzip_static_module \
       --without-http_auth_basic_module --with-cc-opt=-Wno-error \
    && make -j2 && make install \
    && rm -rf /tmp/nginx-1.9.4*
# auth_basic is unused by this fixture; disabling it avoids the obsolete
# crypt_data.current_salt member when compiling unmodified 1.9.4 on modern glibc.
CMD ["nginx", "-g", "daemon off;"]
