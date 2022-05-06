variable "ROCKPOOL_REPO" {
    default = "https://github.com/yusufhm/rockpool"
}

variable "K3S_VERSION" {
    default = "v1.21.11-k3s1"
}

group "default" {
    targets = ["k3s"]
}

target "k3s" {
    dockerfile = "Dockerfile.k3s"
    tags = ["ghcr.io/yusufhm/rockpool/k3s:latest"]
    labels = {"org.opencontainers.image.source": "${ROCKPOOL_REPO}"}
    platforms = ["linux/amd64", "linux/arm64"]
    args = {
        K3S_VERSION = "${K3S_VERSION}"
    }
}
