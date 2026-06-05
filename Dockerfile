FROM fedora:latest

RUN dnf install -y glibc libstdc++ libgcc git && dnf clean all

RUN useradd -m -u 1000 playground

COPY rux /usr/local/bin/rux
COPY entrypoint.sh /entrypoint.sh

RUN chmod 755 /entrypoint.sh

# Pre-install Std & Linux into global cache (as playground user)
USER playground
RUN mkdir -p /tmp/prep/Src && cd /tmp/prep && \
    printf '[Package]\nName = "prep"\nVersion = "0.1.0"\n\n[Dependencies]\nStd = "*"\nLinux = "*"\n' > Rux.toml && \
    echo 'func Main()->int{return 0}' > Src/Main.rux && \
    rux install && cd / && rm -rf /tmp/prep
WORKDIR /home/playground

ENTRYPOINT ["/entrypoint.sh"]
