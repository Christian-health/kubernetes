/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"sync"

	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/scheme"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/component-base/version"
)

const (
	flagMatchBinaryVersion = "match-server-version"
)
/*
	如下代码定义了一个名为MatchVersionFlags的结构体。该结构体用于设置“匹配服务器版本”的功能。
	MatchVersionFlags结构体包含以下字段：
		- Delegate：一个genericclioptions.RESTClientGetter类型的字段，用于获取REST客户端。这个字段允许将实际的REST客户端获取逻辑委托给其他对象。
		- RequireMatchedServerVersion：一个布尔类型的字段，表示是否要求匹配服务器版本。如果设置为true，则在与服务器通信之前，客户端将检查其版本是否与服务器版本匹配。
		- checkServerVersion：一个sync.Once类型的字段，用于确保只检查一次服务器版本。
		- matchesServerVersionErr：一个error类型的字段，用于存储检查服务器版本时发生的错误。
	MatchVersionFlags结构体的目的是为了在Kubernetes客户端中提供一种机制，用于检查客户端版本与服务器版本是否匹配。通过设置RequireMatchedServerVersion字段为true，可以启用这种匹配功能，并在与服务器通信之前执行版本检查。这可以确保客户端与服务器之间的版本兼容性，以避免潜在的兼容性问题。
*/
// MatchVersionFlags is for setting the "match server version" function.
type MatchVersionFlags struct {
	Delegate genericclioptions.RESTClientGetter

	RequireMatchedServerVersion bool
	checkServerVersion          sync.Once
	matchesServerVersionErr     error
}

var _ genericclioptions.RESTClientGetter = &MatchVersionFlags{}

func (f *MatchVersionFlags) checkMatchingServerVersion() error {
	f.checkServerVersion.Do(func() {
		if !f.RequireMatchedServerVersion {
			return
		}
		discoveryClient, err := f.Delegate.ToDiscoveryClient()
		if err != nil {
			f.matchesServerVersionErr = err
			return
		}
		f.matchesServerVersionErr = discovery.MatchesServerVersion(version.Get(), discoveryClient)
	})

	return f.matchesServerVersionErr
}

// ToRESTConfig implements RESTClientGetter.
// Returns a REST client configuration based on a provided path
// to a .kubeconfig file, loading rules, and config flag overrides.
// Expects the AddFlags method to have been called.
func (f *MatchVersionFlags) ToRESTConfig() (*rest.Config, error) {
	if err := f.checkMatchingServerVersion(); err != nil {
		return nil, err
	}
	clientConfig, err := f.Delegate.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	// TODO we should not have to do this.  It smacks of something going wrong.
	setKubernetesDefaults(clientConfig)
	return clientConfig, nil
}

func (f *MatchVersionFlags) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return f.Delegate.ToRawKubeConfigLoader()
}

func (f *MatchVersionFlags) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	if err := f.checkMatchingServerVersion(); err != nil {
		return nil, err
	}
	return f.Delegate.ToDiscoveryClient()
}

// ToRESTMapper returns a mapper.
func (f *MatchVersionFlags) ToRESTMapper() (meta.RESTMapper, error) {
	if err := f.checkMatchingServerVersion(); err != nil {
		return nil, err
	}
	return f.Delegate.ToRESTMapper()
}

func (f *MatchVersionFlags) AddFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&f.RequireMatchedServerVersion, flagMatchBinaryVersion, f.RequireMatchedServerVersion, "Require server version to match client version")
}

func NewMatchVersionFlags(delegate genericclioptions.RESTClientGetter) *MatchVersionFlags {
	return &MatchVersionFlags{
		Delegate: delegate,
	}
}

// setKubernetesDefaults sets default values on the provided client config for accessing the
// Kubernetes API or returns an error if any of the defaults are impossible or invalid.
// TODO this isn't what we want.  Each clientset should be setting defaults as it sees fit.
func setKubernetesDefaults(config *rest.Config) error {
	// TODO remove this hack.  This is allowing the GetOptions to be serialized.
	config.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"}

	if config.APIPath == "" {
		config.APIPath = "/api"
	}
	if config.NegotiatedSerializer == nil {
		// This codec factory ensures the resources are not converted. Therefore, resources
		// will not be round-tripped through internal versions. Defaulting does not happen
		// on the client.
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}
	return rest.SetKubernetesDefaults(config)
}
