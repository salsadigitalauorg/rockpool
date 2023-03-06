variable "ROCKPOOL_REPO" {
    default = "https://github.com/salsadigitalauorg/rockpool"
}

variable "ROCKPOOL_IMAGES_REPO" {
    default = "ghcr.io/salsadigitalauorg/rockpool"
}

variable "K3S_VERSION_1_23" {
    default = "v1.23.16-k3s1"
}

variable "K3S_VERSION_1_24" {
    default = "v1.24.10-k3s1"
}

variable "K3S_VERSION_1_25" {
    default = "v1.25.6-k3s1"
}

variable "NFS_GANESHA_VERSION" {
    default = "V4.0.8"
}

group "k3s" {
    targets = ["k3s-1_23", "k3s-1_24", "k3s-1_25"]
}

target "k3s-base" {
    dockerfile = "Dockerfile.k3s"
    labels = {"org.opencontainers.image.source": "${ROCKPOOL_REPO}"}
    platforms = ["linux/amd64", "linux/arm64"]
}

target "k3s-1_23" {
    inherits = ["k3s-base"]
    tags = [
        "${ROCKPOOL_IMAGES_REPO}/k3s:${K3S_VERSION_1_23}",
        "${ROCKPOOL_IMAGES_REPO}/k3s:v1.23",
        "${ROCKPOOL_IMAGES_REPO}/k3s:latest"
    ]
    args = {
        K3S_VERSION = "${K3S_VERSION_1_23}"
    }
}

target "k3s-1_24" {
    inherits = ["k3s-base"]
    tags = [
        "${ROCKPOOL_IMAGES_REPO}/k3s:${K3S_VERSION_1_24}",
        "${ROCKPOOL_IMAGES_REPO}/k3s:v1.24"
    ]
    args = {
        K3S_VERSION = "${K3S_VERSION_1_24}"
    }
}

target "k3s-1_25" {
    inherits = ["k3s-base"]
    tags = [
        "${ROCKPOOL_IMAGES_REPO}/k3s:${K3S_VERSION_1_25}",
        "${ROCKPOOL_IMAGES_REPO}/k3s:v1.25"
    ]
    args = {
        K3S_VERSION = "${K3S_VERSION_1_25}"
    }
}

target "nfs-provisioner" {
    dockerfile = "Dockerfile.nfs-provisioner"
    tags = ["${ROCKPOOL_IMAGES_REPO}/nfs-provisioner:latest"]
    labels = {"org.opencontainers.image.source": "${ROCKPOOL_REPO}"}
    platforms = ["linux/amd64", "linux/arm64"]
    args = {
        NFS_GANESHA_VERSION = "${NFS_GANESHA_VERSION}"
    }
}
