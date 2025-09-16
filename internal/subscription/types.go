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

// Note: ClashProxy and ClashConfig structures are now defined in
// internal/subscription/clash/model/clash package for better organization
// and more comprehensive field support.
