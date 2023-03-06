package rockpool

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/platform/templates"

	log "github.com/sirupsen/logrus"
)

var HarborSecretManifest string
var HarborCaCrtFile string

var ingressNginxInstaller = helm.Installer{
	Info:        "installing ingress-nginx",
	Namespace:   "ingress-nginx",
	ReleaseName: "ingress-nginx",
	Chart:       "https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-3.40.0/ingress-nginx-3.40.0.tgz",
	Args: []string{
		"--create-namespace", "--wait",
		"--set", "controller.config.ssl-redirect=false",
		"--set", "controller.config.proxy-body-size=8m",
		"--set", "server-name-hash-bucket-size=128",
	},
}

func FetchHarborCerts() {
	cn := platform.ControllerClusterName()
	logger := log.WithField("clusterName", cn)
	logger.Info("fetching harbor certificates")

	certBytes, _ := kube.GetSecret(cn, "harbor", "harbor-harbor-ingress", "")
	certData := struct {
		Data map[string]string `json:"data"`
	}{}
	json.Unmarshal(certBytes, &certData)

	secretManifest, err := templates.Render("harbor-cert.yml.tmpl", certData, "")
	if err != nil {
		logger.WithError(err).Fatal("error rendering harbor cert template")
	}
	logger.WithField("secret", secretManifest).Debug("generated harbor cert")

	cacrt := certData.Data["ca.crt"]
	decoded, err := base64.URLEncoding.DecodeString(cacrt)
	if err != nil {
		logger.WithError(err).Fatal("error decoding ca.crt")
	}
	caCrtFile, err := templates.Render("harbor-ca.crt.tmpl", string(decoded), "")
	if err != nil {
		logger.WithError(err).Fatal("error rendering harbor ca.crt template")
	}
	logger.WithField("certificate", caCrtFile).Debug("generated harbor ca.crt")

	HarborSecretManifest = secretManifest
	HarborCaCrtFile = caCrtFile
}

func InstallHarborCerts(cn string) {
	if cn == platform.ControllerClusterName() {
		return
	}
	logger := log.WithField("clusterName", cn)
	logger.Info("installing harbor certificates on target")

	exists, c := k3d.ClusterExists(cn)
	if !exists {
		logger.Warn("cluster does not exist")
		return
	}

	if err := kube.Apply(cn, "lagoon", HarborSecretManifest, true); err != nil {
		logger.WithField("secret", HarborSecretManifest).WithError(err).
			Fatal("error applying ca.crt")
	}

	// Add the cert to the nodes.
	clusterUpdated := false
	for _, n := range c.Nodes {
		if n.Role == "loadbalancer" {
			continue
		}

		caCrtFileOut, _ := docker.Exec(n.Name, "ls /etc/ssl/certs/harbor-cert.crt").Output()
		if strings.Trim(string(caCrtFileOut), "\n") == "/etc/ssl/certs/harbor-cert.crt" {
			continue
		}

		// Add harbor's ca.crt to the target.
		destCaCrt := fmt.Sprintf("%s:/etc/ssl/certs/harbor-cert.crt", n.Name)
		_, err := docker.Cp(HarborCaCrtFile, destCaCrt)
		if err != nil {
			logger.WithFields(log.Fields{
				"src":  HarborCaCrtFile,
				"dest": destCaCrt,
			}).WithError(command.GetMsgFromCommandError(err)).
				Fatal("error copying ca.crt")
		}
		clusterUpdated = true
	}

	if clusterUpdated {
		k3d.ClusterRestart(c.Name)
	}

	// Patch lagoon-remote-lagoon-build-deploy to add the cert secret.
	patchFile, err := templates.Render("patch-lagoon-remote-lagoon-build-deploy.yaml", nil, "")
	if err != nil {
		logger.WithError(err).
			Fatal("error rendering the build deploy patch file")
	}
	_, err = kube.Patch(cn, "lagoon", "deployment", "lagoon-remote-lagoon-build-deploy", patchFile)
	if err != nil {
		logger.WithField("patchFile", patchFile).
			WithError(command.GetMsgFromCommandError(err)).
			Fatal("error patching the lagoon-build-deploy deployment")
	}
}

// AddHarborHostEntries adds host entries to the target nodes.
func AddHarborHostEntries(cn string) {
	if cn == platform.ControllerClusterName() {
		return
	}
	logger := log.WithField("clusterName", cn)
	logger.Info("adding harbor host entries on target")

	exists, c := k3d.ClusterExists(cn)
	if !exists {
		logger.Warn("cluster does not exist")
		return
	}

	entry := fmt.Sprintf("%s\tharbor.lagoon.%s", k3d.ControllerIP(), platform.Hostname())
	entryCmdStr := fmt.Sprintf("echo '%s' >> /etc/hosts", entry)
	for _, n := range c.Nodes {
		if n.Role == "loadbalancer" {
			continue
		}

		hostsContent, _ := docker.Exec(n.Name, "cat /etc/hosts").Output()
		if !strings.Contains(string(hostsContent), entry) {
			logger.WithFields(log.Fields{
				"node":  n.Name,
				"entry": entry,
			}).Debug("adding harbor host entry")
			err := docker.Exec(n.Name, entryCmdStr).Run()
			if err != nil {
				logger.WithFields(log.Fields{
					"node":  n.Name,
					"entry": entry,
				}).WithError(command.GetMsgFromCommandError(err)).
					Fatal("error adding harbor host entry")
			}
		}
	}
}

// ConfigureTargetCoreDNS adds DNS records to targets for the required services.
var ConfigureTargetCoreDNS = func(logger *log.Entry) bool {
	cn := logger.Data["cluster"].(string)
	cm := kube.GetConfigMap(cn, "kube-system", "coredns")
	corednsCm := CoreDNSConfigMap{}
	err := json.Unmarshal(cm, &corednsCm)
	if err != nil {
		logger.WithError(err).Fatal("error parsing CoreDNS configmap")
	}
	for _, h := range []string{"harbor", "broker", "ssh", "api", "gitea"} {
		entry := fmt.Sprintf("%s %s.lagoon.%s\n", k3d.ControllerIP(), h, platform.Hostname())
		if !strings.Contains(corednsCm.Data.NodeHosts, entry) {
			corednsCm.Data.NodeHosts += entry
		}
	}

	cm, err = json.Marshal(corednsCm)
	if err != nil {
		logger.WithError(err).Fatal("error encoding CoreDNS configmap")
	}

	kube.Replace(cn, "kube-system", "coredns", string(cm))

	logger.Info("restarting coredns")
	err = kube.Cmd(cn, "kube-system", "rollout", "restart",
		"deployment/coredns").RunProgressive()
	if err != nil {
		logger.WithError(err).Fatal("CoreDNS restart failed")
	}
	return true
}
