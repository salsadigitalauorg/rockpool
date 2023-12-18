package config

import (
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

var v = viper.New()

var C *Config

func Initialise() {
	v.SetConfigName("rockpool")
	v.AddConfigPath("$HOME/.rockpool")
	v.AddConfigPath(".")
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	hasConfig := true
	err := v.ReadInConfig()
	if err != nil {
		hasConfig = false
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			log.Debug("no config file found")
		} else {
			// Config file was found but another error was produced.
			log.WithError(err).Panic("failed to read config file")
		}
	}
	log.WithField("config file", v.ConfigFileUsed()).Debug()

	if hasConfig {
		err := v.Unmarshal(&C)
		if err != nil {
			log.WithError(err).Panic("failed to unmarshal config")
		}
	} else {
		C = &Config{
			Name:   "rockpool",
			Domain: "rockpool.local",
			Clusters: Clusters{
				Single:   true,
				Provider: ClusterProviderKind,
			},
		}
		log.WithField("config", C).Debug("initialised default config")
	}
}

func (cc *ClusterConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"Kubeconfig": cc.Kubeconfig,
		"Context":    cc.Context,
	}
}

func (cs *Clusters) ToMap() map[string]interface{} {
	remotes := make([]map[string]interface{}, len(cs.Remotes))
	for i, r := range cs.Remotes {
		remotes[i] = r.ToMap()
	}
	return map[string]interface{}{
		"KubernetesVersion": cs.KubernetesVersion,
		"Single":            cs.Single,
		"Provider":          cs.Provider,
		"Core":              cs.Core.ToMap(),
		"Remotes":           remotes,
	}
}

func (cc *ComponentConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"ChartValues": cc.ChartValues,
	}
}

func (c *Config) ToMap() map[string]interface{} {
	components := make(map[string]interface{})
	for k, v := range c.Components {
		components[k] = v.ToMap()
	}
	return map[string]interface{}{
		"Name":       C.Name,
		"Domain":     C.Domain,
		"Clusters":   C.Clusters.ToMap(),
		"Components": components,
	}
}
