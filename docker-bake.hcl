variable "ROCKPOOL_REPO" {
    default = "https://github.com/salsadigitalauorg/rockpool"
}

variable "K3S_VERSION" {
    default = "v1.21.11-k3s1"
}

group "default" {
    targets = ["k3s"]
}

target "k3s" {
    dockerfile = "Dockerfile.k3s"
    tags = ["ghcr.io/salsadigitalauorg/rockpool/k3s:latest"]
    labels = {"org.opencontainers.image.source": "${ROCKPOOL_REPO}"}
    platforms = ["linux/amd64", "linux/arm64"]
    args = {
        K3S_VERSION = "${K3S_VERSION}"
    }
}

target "nfs-provisioner" {
    dockerfile = "Dockerfile.nfs-provisioner"
    tags = ["ghcr.io/salsadigitalauorg/rockpool/nfs-provisioner:latest"]
    labels = {"org.opencontainers.image.source": "${ROCKPOOL_REPO}"}
    platforms = ["linux/amd64", "linux/arm64"]
}
