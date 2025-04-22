# Use the base image for Ubuntu 24.04 Noble
ARG DCM_BASE_IMAGE=22.04
FROM ubuntu:${DCM_BASE_IMAGE}

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64

# Update the package manager and install required dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    gcc \
    g++ \
    libc6-dev \
    make \
    pkg-config \
    zlib1g-dev \
    vim \
    wget \
    build-essential

# Install Go (you can replace the version as needed)
RUN curl -fsSL https://golang.org/dl/go1.23.4.linux-amd64.tar.gz | tar -C /usr/local -xzf - && \
    ln -s /usr/local/go/bin/go /usr/bin/go

# Set up the workspace
WORKDIR /device-config-manager

# Copy the source code into the container
COPY ../../device-config-manager .
RUN go mod tidy && go mod download

CMD ["bash"]
