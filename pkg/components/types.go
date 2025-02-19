package components

import (
	"context"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

// TemplateData contains all the data that can be used in templates
type TemplateData struct {
	// Component is the original component configuration
	Component config.ComponentConfig
	// Hostname is the cluster hostname
	Hostname string
	// Arch is the system architecture (e.g., "arm64", "amd64")
	Arch string
	// LagoonVersion is the version of Lagoon to use
	LagoonVersion string
	// RabbitMQPassword is the password for RabbitMQ
	RabbitMQPassword string
	// Name is the component name
	Name string
	// TargetId is an identifier for the target
	TargetId string
	// Domain is the domain name
	Domain string
	// VmIp is the IP address of the VM
	VmIp string
	// Data contains any additional template data
	Data map[string]interface{}
}

// Manager handles component lifecycle operations
type Manager interface {
	// Install installs a component
	Install(ctx context.Context, component config.ComponentConfig) error
	// Upgrade upgrades a component
	Upgrade(ctx context.Context, component config.ComponentConfig) error
	// Uninstall removes a component
	Uninstall(ctx context.Context, component config.ComponentConfig) error
	// IsInstalled checks if a component is installed
	IsInstalled(ctx context.Context, component config.ComponentConfig) (bool, error)
}

// ComponentManager implements the Manager interface
type ComponentManager struct {
	config      *config.Config
	clusterName string
}
