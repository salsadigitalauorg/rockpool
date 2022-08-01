variable "ROCKPOOL_REPO" {
    default = "https://github.com/salsadigitalauorg/rockpool"
}

variable "K3S_VERSION_1_21" {
    default = "v1.21.14-k3s1"
}

variable "K3S_VERSION_1_22" {
    default = "v1.22.12-k3s1"
}

variable "K3S_VERSION_1_23" {
    default = "v1.23.9-k3s1"
}

variable "K3S_VERSION_1_24" {
    default = "v1.24.3-k3s1"
}

group "k3s" {
    targets = ["k3s-1_21", "k3s-1_22", "k3s-1_23", "k3s-1_24"]
}

target "k3s-base" {
    dockerfile = "Dockerfile.k3s"
    labels = {"org.opencontainers.image.source": "${ROCKPOOL_REPO}"}
    platforms = ["linux/amd64", "linux/arm64"]
}

target "k3s-1_21" {
    inherits = ["k3s-base"]
    tags = ["ghcr.io/salsadigitalauorg/rockpool/k3s:${K3S_VERSION_1_21}"]
    args = {
        K3S_VERSION = "${K3S_VERSION_1_21}"
    }
}

target "k3s-1_22" {
    inherits = ["k3s-base"]
    tags = ["ghcr.io/salsadigitalauorg/rockpool/k3s:${K3S_VERSION_1_22}"]
    args = {
        K3S_VERSION = "${K3S_VERSION_1_22}"
    }
}

target "k3s-1_23" {
    inherits = ["k3s-base"]
    tags = ["ghcr.io/salsadigitalauorg/rockpool/k3s:${K3S_VERSION_1_23}"]
    args = {
        K3S_VERSION = "${K3S_VERSION_1_23}"
    }
}

target "k3s-1_24" {
    inherits = ["k3s-base"]
    tags = ["ghcr.io/salsadigitalauorg/rockpool/k3s:${K3S_VERSION_1_24}"]
    args = {
        K3S_VERSION = "${K3S_VERSION_1_24}"
    }
}

target "nfs-provisioner" {
    dockerfile = "Dockerfile.nfs-provisioner"
    tags = ["ghcr.io/salsadigitalauorg/rockpool/nfs-provisioner:latest"]
    labels = {"org.opencontainers.image.source": "${ROCKPOOL_REPO}"}
    platforms = ["linux/amd64", "linux/arm64"]
}
