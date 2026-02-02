#!/bin/bash
# Build the coop-agent-base image from Ubuntu cloud image
# Usage: ./scripts/build-base-image.sh

set -euo pipefail

IMAGE_NAME="coop-agent-base"
BUILD_CONTAINER="coop-base-build"
SOURCE_IMAGE="images:ubuntu/22.04/cloud"
DESCRIPTION="Coop agent base: Ubuntu 22.04 + Python 3.13 + Go 1.24 + Node 24 + dev tools (no Claude CLI)"

echo "==> Building $IMAGE_NAME from $SOURCE_IMAGE"

# Cleanup any previous build
incus delete "$BUILD_CONTAINER" --force 2>/dev/null || true

# Launch fresh Ubuntu container
echo "==> Launching build container..."
incus launch "$SOURCE_IMAGE" "$BUILD_CONTAINER"

# Wait for container to be ready
echo "==> Waiting for container to start..."
sleep 5

# Run the build script inside the container
echo "==> Installing packages and tools..."
incus exec "$BUILD_CONTAINER" -- bash -ex <<'INSTALL_SCRIPT'

# Switch to faster Berkeley OCF mirror for ARM64 (ports.ubuntu.com is slow)
sed -i 's|http://ports.ubuntu.com/ubuntu-ports|http://mirrors.ocf.berkeley.edu/ubuntu-ports|g' /etc/apt/sources.list

# Enable parallel downloads for apt
echo 'Acquire::Queue-Mode "host";' > /etc/apt/apt.conf.d/99parallel
echo 'Acquire::http::Pipeline-Depth "10";' >> /etc/apt/apt.conf.d/99parallel

# Update and install base packages
apt-get update
apt-get upgrade -y
apt-get install -y \
    build-essential \
    curl \
    wget \
    git \
    vim \
    jq \
    unzip \
    zip \
    htop \
    tree \
    ripgrep \
    fd-find \
    fzf \
    tmux \
    ssh \
    rsync \
    ca-certificates \
    gnupg \
    lsb-release \
    software-properties-common \
    ufw \
    pipx

# Python 3.13 via deadsnakes PPA (pre-built, fast)
add-apt-repository -y ppa:deadsnakes/ppa
apt-get update
apt-get install -y python3.13 python3.13-venv python3.13-dev python3-pip
update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.13 1

# Go 1.24
GO_VERSION="1.24.0"
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-arm64.tar.gz" | tar -C /usr/local -xzf -
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
chmod +x /etc/profile.d/go.sh
# Also add to /etc/environment for non-login shells (incus exec)
sed -i 's|PATH="|PATH="/usr/local/go/bin:|' /etc/environment

# Node.js 24 via NodeSource (system-wide)
curl -fsSL https://deb.nodesource.com/setup_24.x | bash -
apt-get install -y nodejs

# GitHub CLI
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list
apt-get update
apt-get install -y gh

# yq (YAML processor)
YQ_VERSION="v4.44.1"
curl -fsSL "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_arm64" -o /usr/local/bin/yq
chmod +x /usr/local/bin/yq

# Python tools via pipx (system-wide)
PIPX_HOME=/opt/pipx PIPX_BIN_DIR=/usr/local/bin pipx install uv
PIPX_HOME=/opt/pipx PIPX_BIN_DIR=/usr/local/bin pipx install ruff

# Go tools
export PATH=$PATH:/usr/local/go/bin
GOPATH=/opt/go go install golang.org/x/tools/gopls@latest
mv /opt/go/bin/gopls /usr/local/bin/

# NOTE: Claude Code CLI is installed per-container via cloud-init, not in base image
# This avoids OOM during base image build and keeps the image smaller

# Verify installations
echo "==> Verifying installations..."
python3 --version
/usr/local/go/bin/go version
node --version
npm --version
gh --version | head -1

# Lock ubuntu user and change UID to avoid conflict with agent (UID 1000)
usermod -u 2000 ubuntu
groupmod -g 2000 ubuntu
passwd -l ubuntu
usermod -s /usr/sbin/nologin ubuntu

# Cleanup
apt-get autoremove -y
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

echo "==> Build complete!"
INSTALL_SCRIPT

# Stop the container
echo "==> Stopping build container..."
incus stop "$BUILD_CONTAINER"

# Publish as image
echo "==> Publishing image as $IMAGE_NAME..."
incus publish "$BUILD_CONTAINER" --alias "$IMAGE_NAME" --reuse \
    description="$DESCRIPTION"

# Cleanup build container
echo "==> Cleaning up..."
incus delete "$BUILD_CONTAINER"

# Show result
echo "==> Done! Image published:"
incus image list "$IMAGE_NAME"
