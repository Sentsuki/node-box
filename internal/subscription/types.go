// Package subscription provides subscription data processing functionality
// for different proxy subscription formats including Clash and SingBox.
// It handles parsing, conversion, and filtering of proxy nodes from various sources.
package subscription

import "errors"

// Subscription package errors
var (
	ErrUnsupportedProxyType   = errors.New("unsupported proxy type")
	ErrInvalidClashConfig     = errors.New("invalid Clash configuration")
	ErrInvalidSingBoxConfig   = errors.New("invalid SingBox configuration")
	ErrMissingOutbounds       = errors.New("missing outbounds field in configuration")
	ErrInvalidOutboundsFormat = errors.New("invalid outbounds field format")
	ErrPortConversionFailed   = errors.New("port conversion failed")
	ErrInvalidAlterId         = errors.New("invalid alterId value")
)

// Node represents a unified node data structure using any instead of interface{}.
// It provides a flexible way to store proxy node configuration data
// that can be serialized to different output formats.
type Node map[string]any

// Processor defines the interface for processing different types of subscription data.
// Implementations handle specific subscription formats and convert them to unified Node format.
type Processor interface {
	// Process parses subscription data and returns a list of proxy nodes.
	// It takes raw subscription data as bytes and returns processed nodes or an error.
	Process(data []byte) ([]Node, error)
}

// ClashProxy represents a Clash proxy structure for parsing Clash format subscriptions.
// It contains all the fields commonly found in Clash proxy configurations.
type ClashProxy struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`
	Server         string            `yaml:"server"`
	Port           string            `yaml:"port"`
	Cipher         string            `yaml:"cipher,omitempty"`
	Password       string            `yaml:"password,omitempty"`
	UUID           string            `yaml:"uuid,omitempty"`
	AlterId        string            `yaml:"alterId,omitempty"`
	Security       string            `yaml:"security,omitempty"`
	Network        string            `yaml:"network,omitempty"`
	WSPath         string            `yaml:"ws-path,omitempty"`
	WSHeaders      map[string]string `yaml:"ws-headers,omitempty"`
	TLS            bool              `yaml:"tls,omitempty"`
	SkipCertVerify bool              `yaml:"skip-cert-verify,omitempty"`
	ServerName     string            `yaml:"servername,omitempty"`
	UDP            bool              `yaml:"udp,omitempty"`
}

// ClashConfig represents the main Clash configuration structure.
// It contains the list of proxy configurations parsed from Clash subscription data.
type ClashConfig struct {
	Proxies []ClashProxy `yaml:"proxies"`
}
