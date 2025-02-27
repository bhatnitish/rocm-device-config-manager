FROM registry.access.redhat.com/ubi9/ubi-minimal:9.4

LABEL maintainer="AMD Inc"
LABEL OS="registry.access.redhat.com/ubi9/ubi-minimal:9.4"
# unused for now revisit
ARG ROCM_VERSION=6.4
ARG AMDGPU_VERSION=6.4

ADD rhelrepofiles/amdgpu.repo /etc/yum.repos.d/amdgpu.repo 
ADD rhelrepofiles/rocm.repo /etc/yum.repos.d/rocm.repo

RUN microdnf clean all && microdnf install -y sudo procps file libcap-devel libdrm-amdgpu vim net-tools kmod && microdnf install -y amd-smi-lib && rm -rf /var/cache/yum && rm -rf /var/cache/dnf && microdnf clean all

ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/home/amd/bin/:/opt/rocm/bin/
RUN mkdir -p /home/amd/bin/
RUN mkdir -p /home/amd/lib/

ADD ./smilib/ /home/amd/lib/
ENV LD_LIBRARY_PATH=/home/amd/lib/
ADD ./device-config-manager /home/amd/bin/server
RUN mkdir -p /home/amd/tools/

ADD ./entrypoint.sh /home/amd/tools/entrypoint.sh
RUN chmod +x /home/amd/tools/entrypoint.sh

ENTRYPOINT ["/home/amd/tools/entrypoint.sh"]
